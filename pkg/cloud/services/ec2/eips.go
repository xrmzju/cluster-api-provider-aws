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

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/pkg/errors"
	"sigs.k8s.io/cluster-api-provider-aws/api/v1alpha2"
	"sigs.k8s.io/cluster-api-provider-aws/pkg/cloud/awserrors"
	"sigs.k8s.io/cluster-api-provider-aws/pkg/cloud/filter"
	"sigs.k8s.io/cluster-api-provider-aws/pkg/cloud/services/wait"
	"sigs.k8s.io/cluster-api-provider-aws/pkg/cloud/tags"
)

func (s *Service) getOrAllocateAddress(role string) (string, error) {
	out, err := s.describeAddresses(role)
	if err != nil {
		return "", errors.Wrap(err, "failed to query addresses")
	}

	// TODO: better handle multiple addresses returned
	for _, address := range out.Addresses {
		if address.AssociationId == nil {
			return aws.StringValue(address.AllocationId), nil
		}
	}

	return s.allocateAddress(role)
}

func (s *Service) allocateAddress(role string) (string, error) {
	out, err := s.scope.EC2.AllocateAddress(&ec2.AllocateAddressInput{
		Domain: aws.String("vpc"),
	})

	if err != nil {
		return "", errors.Wrap(err, "failed to create Elastic IP address")
	}

	if err := wait.WaitForWithRetryable(wait.NewBackoff(), func() (bool, error) {
		if err := tags.Apply(&tags.ApplyParams{
			EC2Client: s.scope.EC2,
			BuildParams: v1alpha2.BuildParams{
				ClusterName: s.scope.Name(),
				ResourceID:  *out.AllocationId,
				Lifecycle:   v1alpha2.ResourceLifecycleOwned,
				Name:        aws.String(fmt.Sprintf("%s-eip-%s", s.scope.Name(), role)),
				Role:        aws.String(role),
			},
		}); err != nil {
			return false, err
		}
		return true, nil
	}, awserrors.EIPNotFound); err != nil {
		return "", errors.Wrapf(err, "failed to tag elastic IP %q", aws.StringValue(out.AllocationId))
	}

	return aws.StringValue(out.AllocationId), nil
}

func (s *Service) describeAddresses(role string) (*ec2.DescribeAddressesOutput, error) {
	x := []*ec2.Filter{filter.EC2.Cluster(s.scope.Name())}
	if role != "" {
		x = append(x, filter.EC2.ProviderRole(role))
	}

	return s.scope.EC2.DescribeAddresses(&ec2.DescribeAddressesInput{
		Filters: x,
	})
}

func (s *Service) releaseAddresses() error {
	out, err := s.scope.EC2.DescribeAddresses(&ec2.DescribeAddressesInput{
		Filters: []*ec2.Filter{filter.EC2.Cluster(s.scope.Name())},
	})

	if err != nil {
		return errors.Wrapf(err, "failed to describe elastic IPs %q", err)
	}

	for _, ip := range out.Addresses {
		if ip.AssociationId != nil {
			return errors.Errorf("failed to release elastic IP %q with allocation ID %q: Still associated with association ID %q", *ip.PublicIp, *ip.AllocationId, *ip.AssociationId)
		}

		err := wait.WaitForWithRetryable(wait.NewBackoff(), func() (bool, error) {
			_, err := s.scope.EC2.ReleaseAddress(&ec2.ReleaseAddressInput{AllocationId: ip.AllocationId})
			if err != nil {
				return false, err
			}

			return true, nil
		}, awserrors.AuthFailure, awserrors.InUseIPAddress)
		if err != nil {
			return errors.Wrapf(err, "failed to release ElasticIP %q", *ip.AllocationId)
		}

		s.scope.Info("released ElasticIP", "eip", *ip.PublicIp, "allocation-id", *ip.AllocationId)
	}
	return nil
}
