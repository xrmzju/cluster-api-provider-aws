/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package ec2

import (
	"fmt"

	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/cluster-api-provider-aws/pkg/cloud/services/wait"

	"sigs.k8s.io/cluster-api-provider-aws/pkg/record"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/pkg/errors"
	"sigs.k8s.io/cluster-api-provider-aws/api/v1alpha2"
	"sigs.k8s.io/cluster-api-provider-aws/pkg/cloud/awserrors"
	"sigs.k8s.io/cluster-api-provider-aws/pkg/cloud/converters"
	"sigs.k8s.io/cluster-api-provider-aws/pkg/cloud/filter"
	"sigs.k8s.io/cluster-api-provider-aws/pkg/cloud/tags"
)

const (
	defaultVPCCidr = "10.0.0.0/16"
)

func (s *Service) reconcileVPC() error {
	s.scope.V(2).Info("Reconciling VPC")

	vpc, err := s.describeVPC()
	if awserrors.IsNotFound(err) {
		// Create a new managed vpc.
		vpc, err = s.createVPC()
		if err != nil {
			return errors.Wrap(err, "failed to create new vpc")
		}

	} else if err != nil {
		return errors.Wrap(err, "failed to describe VPCs")
	}

	if vpc.IsUnmanaged(s.scope.Name()) {
		vpc.DeepCopyInto(s.scope.VPC())
		s.scope.V(2).Info("Working on unmanaged VPC", "vpc-id", vpc.ID)
		return nil
	}

	// Make sure attributes are configured
	if err := wait.WaitForWithRetryable(wait.NewBackoff(), func() (bool, error) {
		if err := tags.Ensure(vpc.Tags, &tags.ApplyParams{
			EC2Client:   s.scope.EC2,
			BuildParams: s.getVPCTagParams(vpc.ID),
		}); err != nil {
			return false, err
		}
		return true, nil
	}, awserrors.VPCNotFound); err != nil {
		record.Warnf(s.scope.Cluster, "FailedTagVPC", "Failed to tag managed VPC %q: %v", vpc.ID, err)
		return errors.Wrapf(err, "failed to tag vpc %q", vpc.ID)
	}

	// Make sure attributes are configured
	if err := wait.WaitForWithRetryable(wait.NewBackoff(), func() (bool, error) {
		if err := s.ensureManagedVPCAttributes(vpc); err != nil {
			return false, err
		}
		return true, nil
	}, awserrors.VPCNotFound); err != nil {
		return errors.Wrapf(err, "failed to to set vpc attributes for %q", vpc.ID)
	}

	vpc.DeepCopyInto(s.scope.VPC())
	s.scope.V(2).Info("Working on managed VPC", "vpc-id", vpc.ID)
	return nil
}

func (s *Service) ensureManagedVPCAttributes(vpc *v1alpha2.VPCSpec) error {
	var errs []error

	// Cannot get or set both attributes at the same time.
	descAttrInput := &ec2.DescribeVpcAttributeInput{
		VpcId:     aws.String(vpc.ID),
		Attribute: aws.String("enableDnsHostnames"),
	}
	vpcAttr, err := s.scope.EC2.DescribeVpcAttribute(descAttrInput)
	if err != nil {
		errs = append(errs, errors.Wrap(err, "failed to describe enableDnsHostnames vpc attribute"))
	} else if !aws.BoolValue(vpcAttr.EnableDnsHostnames.Value) {
		attrInput := &ec2.ModifyVpcAttributeInput{
			VpcId:              aws.String(vpc.ID),
			EnableDnsHostnames: &ec2.AttributeBooleanValue{Value: aws.Bool(true)},
		}
		if _, err := s.scope.EC2.ModifyVpcAttribute(attrInput); err != nil {
			errs = append(errs, errors.Wrap(err, "failed to set enableDnsHostnames vpc attribute"))
		}
	}

	descAttrInput = &ec2.DescribeVpcAttributeInput{
		VpcId:     aws.String(vpc.ID),
		Attribute: aws.String("enableDnsSupport"),
	}
	vpcAttr, err = s.scope.EC2.DescribeVpcAttribute(descAttrInput)
	if err != nil {
		errs = append(errs, errors.Wrap(err, "failed to describe enableDnsSupport vpc attribute"))
	} else if !aws.BoolValue(vpcAttr.EnableDnsSupport.Value) {
		attrInput := &ec2.ModifyVpcAttributeInput{
			VpcId:            aws.String(vpc.ID),
			EnableDnsSupport: &ec2.AttributeBooleanValue{Value: aws.Bool(true)},
		}
		if _, err := s.scope.EC2.ModifyVpcAttribute(attrInput); err != nil {
			errs = append(errs, errors.Wrap(err, "failed to set enableDnsSupport vpc attribute"))
		}
	}

	if len(errs) > 0 {
		record.Warnf(s.scope.Cluster, "FailedSetVPCAttributes", "Failed to set managed VPC attributes for %q: %v", vpc.ID, err)
		return kerrors.NewAggregate(errs)
	}

	record.Eventf(s.scope.Cluster, "SuccessfulSetVPCAttributes", "Set managed VPC attributes for %q", vpc.ID, err)
	return nil
}

func (s *Service) createVPC() (*v1alpha2.VPCSpec, error) {
	if s.scope.VPC().IsUnmanaged(s.scope.Name()) {
		return nil, errors.Errorf("cannot create a managed vpc in unmanaged mode")
	}

	if s.scope.VPC().CidrBlock == "" {
		s.scope.VPC().CidrBlock = defaultVPCCidr
	}

	input := &ec2.CreateVpcInput{
		CidrBlock: aws.String(s.scope.VPC().CidrBlock),
	}

	out, err := s.scope.EC2.CreateVpc(input)
	if err != nil {
		record.Warnf(s.scope.Cluster, "FailedCreateVPC", "Failed to create new managed VPC: %v", err)
		return nil, errors.Wrap(err, "failed to create vpc")
	}

	record.Eventf(s.scope.Cluster, "SuccessfulCreateVPC", "Created new managed VPC %q", *out.Vpc.VpcId)
	s.scope.V(2).Info("Created new VPC with cidr", "vpc-id", *out.Vpc.VpcId, "cidr-block", *out.Vpc.CidrBlock)

	// TODO: we should attempt to record the VPC ID as soon as possible by setting s.scope.VPC().ID
	// however, the logic used for determining managed vs unmanaged VPCs relies on the tags and will
	// need to be updated to accommodate for the recording of the VPC ID prior to the tagging.

	wReq := &ec2.DescribeVpcsInput{VpcIds: []*string{out.Vpc.VpcId}}
	if err := s.scope.EC2.WaitUntilVpcAvailable(wReq); err != nil {
		return nil, errors.Wrapf(err, "failed to wait for vpc %q", *out.Vpc.VpcId)
	}

	// Apply tags so that we know this is a managed VPC.
	tagParams := s.getVPCTagParams(*out.Vpc.VpcId)
	if err := wait.WaitForWithRetryable(wait.NewBackoff(), func() (bool, error) {
		if err := tags.Apply(&tags.ApplyParams{
			EC2Client:   s.scope.EC2,
			BuildParams: tagParams,
		}); err != nil {
			return false, err
		}
		return true, nil
	}, awserrors.VPCNotFound); err != nil {
		record.Warnf(s.scope.Cluster, "FailedTagVPC", "Failed to tag managed VPC %q: %v", *out.Vpc.VpcId, err)
		return nil, err
	}
	record.Eventf(s.scope.Cluster, "SuccesfulTagVPC", "Tagged managed VPC %q", *out.Vpc.VpcId)

	return &v1alpha2.VPCSpec{
		ID:        *out.Vpc.VpcId,
		CidrBlock: *out.Vpc.CidrBlock,
		Tags:      v1alpha2.Build(tagParams),
	}, nil
}

func (s *Service) deleteVPC() error {
	vpc := s.scope.VPC()

	if vpc.IsUnmanaged(s.scope.Name()) {
		s.scope.V(4).Info("Skipping VPC deletion in unmanaged mode")
		return nil
	}

	vpc, err := s.describeVPC()
	if err != nil {
		if awserrors.IsNotFound(err) {
			// If the VPC does not exist, nothing to do
			return nil
		}
		return err
	}

	input := &ec2.DeleteVpcInput{
		VpcId: aws.String(vpc.ID),
	}

	if _, err := s.scope.EC2.DeleteVpc(input); err != nil {
		// Ignore if it's already deleted
		if code, ok := awserrors.Code(err); ok && code == awserrors.VPCNotFound {
			s.scope.V(4).Info("Skipping VPC deletion, VPC not found")
			return nil
		}
		record.Warnf(s.scope.Cluster, "FailedDeleteVPC", "Failed to delete managed VPC %q: %v", vpc.ID, err)
		return errors.Wrapf(err, "failed to delete vpc %q", vpc.ID)
	}

	s.scope.V(2).Info("Deleted VPC", "vpc-id", vpc.ID)
	record.Eventf(s.scope.Cluster, "SuccessfulDeleteVPC", "Deleted managed VPC %q", vpc.ID)
	return nil
}

func (s *Service) describeVPC() (*v1alpha2.VPCSpec, error) {
	input := &ec2.DescribeVpcsInput{
		Filters: []*ec2.Filter{
			filter.EC2.VPCStates(ec2.VpcStatePending, ec2.VpcStateAvailable),
		},
	}

	if s.scope.VPC().ID == "" {
		// Try to find a previously created and tagged VPC
		input.Filters = append(input.Filters, filter.EC2.Cluster(s.scope.Name()))
	} else {
		input.VpcIds = []*string{aws.String(s.scope.VPC().ID)}
	}

	out, err := s.scope.EC2.DescribeVpcs(input)
	if err != nil {
		if awserrors.IsNotFound(err) {
			return nil, err
		}

		return nil, errors.Wrap(err, "failed to query ec2 for VPCs")
	}

	if len(out.Vpcs) == 0 {
		return nil, awserrors.NewNotFound(errors.Errorf("could not find vpc %q", s.scope.VPC().ID))
	} else if len(out.Vpcs) > 1 {
		return nil, awserrors.NewConflict(errors.Errorf("found more than one vpc with supplied filters. Please clean up extra VPCs: %s", out.GoString()))
	}

	switch *out.Vpcs[0].State {
	case ec2.VpcStateAvailable, ec2.VpcStatePending:
	default:
		return nil, awserrors.NewNotFound(errors.Errorf("could not find available or pending vpc"))
	}

	return &v1alpha2.VPCSpec{
		ID:        *out.Vpcs[0].VpcId,
		CidrBlock: *out.Vpcs[0].CidrBlock,
		Tags:      converters.TagsToMap(out.Vpcs[0].Tags),
	}, nil
}

func (s *Service) getVPCTagParams(id string) v1alpha2.BuildParams {
	name := fmt.Sprintf("%s-vpc", s.scope.Name())

	return v1alpha2.BuildParams{
		ClusterName: s.scope.Name(),
		ResourceID:  id,
		Lifecycle:   v1alpha2.ResourceLifecycleOwned,
		Name:        aws.String(name),
		Role:        aws.String(v1alpha2.CommonRoleTagValue),
	}
}
