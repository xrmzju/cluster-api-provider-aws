# Contributing Guidelines
<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->


- [Contributor License Agreements](#contributor-license-agreements)
- [Finding Things That Need Help](#finding-things-that-need-help)
- [Contributing a Patch](#contributing-a-patch)
- [Backporting a Patch](#backporting-a-patch)
  - [Merge Approval](#merge-approval)
  - [Google Doc Viewing Permissions](#google-doc-viewing-permissions)
  - [Issue and Pull Request Management](#issue-and-pull-request-management)
- [Cloud Provider Developer Guide](#cloud-provider-developer-guide)
  - [Overview](#overview)
  - [Resources](#resources)
  - [Boostrapping](#boostrapping)
  - [A new Machine can be created in a declarative way](#a-new-machine-can-be-created-in-a-declarative-way)
    - [Configurable Machine Setup](#configurable-machine-setup)
      - [GCE Implementation](#gce-implementation)
  - [A specific Machine can be deleted, freeing external resources associated with it.](#a-specific-machine-can-be-deleted-freeing-external-resources-associated-with-it)
  - [A specific Machine can be upgraded or downgraded](#a-specific-machine-can-be-upgraded-or-downgraded)
- [Support Channels](#support-channels)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

Read the following guide if you're interested in contributing to cluster-api.

## Contributor License Agreements

We'd love to accept your patches! Before we can take them, we have to jump a couple of legal hurdles.

Please fill out either the individual or corporate Contributor License Agreement (CLA). More information about the CLA and instructions for signing it [can be found here](https://github.com/kubernetes/community/blob/master/CLA.md).

***NOTE***: Only original source code from you and other people that have signed the CLA can be accepted into the repository.

## Finding Things That Need Help

If you're new to the project and want to help, but don't know where to start, we have a semi-curated list of issues that should not need deep knowledge of the system. [Have a look and see if anything sounds interesting](https://github.com/kubernetes-sigs/cluster-api/issues?q=is%3Aopen+is%3Aissue+label%3A%22good+first+issue%22). Alternatively, read some of the docs on other controllers and try to write your own, file and fix any/all issues that come up, including gaps in documentation!

## Contributing a Patch

1. If you haven't already done so, sign a Contributor License Agreement (see details above).
1. Fork the desired repo, develop and test your code changes.
1. Submit a pull request.

All changes must be code reviewed. Coding conventions and standards are explained in the official [developer docs](https://github.com/kubernetes/community/tree/master/contributors/devel). Expect reviewers to request that you avoid common [go style mistakes](https://github.com/golang/go/wiki/CodeReviewComments) in your PRs.

## Backporting a Patch

Cluster API ships older versions through `release-X.X` branches, usually backports are reserved to critical bug-fixes.
Some release branches might ship with both Go modules and dep (e.g. `release-0.1`), users backporting patches should always make sure
that the vendored Go modules dependencies match the Gopkg.lock and Gopkg.toml ones by running `dep ensure`

### Merge Approval

Cluster API maintainers may add "LGTM" (Looks Good To Me) or an equivalent comment to indicate that a PR is acceptable. Any change requires at least one LGTM.  No pull requests can be merged until at least one Cluster API maintainer signs off with an LGTM.

### Google Doc Viewing Permissions

To gain viewing permissions to google docs in this project, please join either the [kubernetes-dev](https://groups.google.com/forum/#!forum/kubernetes-dev) or [kubernetes-sig-cluster-lifecycle](https://groups.google.com/forum/#!forum/kubernetes-sig-cluster-lifecycle) google group.

### Issue and Pull Request Management

Anyone may comment on issues and submit reviews for pull requests. However, in
order to be assigned an issue or pull request, you must be a member of the
[Kubernetes SIGs](https://github.com/kubernetes-sigs) GitHub organization.

If you are a Kubernetes GitHub organization member, you are eligible for
membership in the Kubernetes SIGs GitHub organization and can request
membership by [opening an issue](https://github.com/kubernetes/org/issues/new?template=membership.md&title=REQUEST%3A%20New%20membership%20for%20%3Cyour-GH-handle%3E)
against the kubernetes/org repo.

However, if you are a member of any of the related Kubernetes GitHub
organizations but not of the Kubernetes org, you will need explicit sponsorship
for your membership request. You can read more about Kubernetes membership and
sponsorship [here](https://github.com/kubernetes/community/blob/master/community-membership.md).

Cluster API maintainers can assign you an issue or pull request by leaving a
`/assign <your Github ID>` comment on the issue or pull request.

## Cloud Provider Developer Guide

### Overview

This document is meant to help OSS contributors implement support for providers (cloud or on-prem).

As part of adding support for a provider (cloud or on-prem), you will need to:

1.  Create tooling that conforms to the Cluster API (described further below)
1.  A machine controller that can run independent of the cluster. This controller should handle the lifecycle of the machines, whether it's run in-cluster or out-cluster.

The machine controller should be able to act on a subset of machines that form a cluster (for example using a label selector).

### Resources

*   [Cluster Management API KEP](https://github.com/kubernetes/enhancements/blob/master/keps/sig-cluster-lifecycle/0003-cluster-api.md)
*   [Cluster type](https://github.com/kubernetes-sigs/cluster-api/blob/master/pkg/apis/deprecated/v1alpha1/cluster_types.go#L40)
*   [Machine type](https://github.com/kubernetes-sigs/cluster-api/blob/master/pkg/apis/deprecated/v1alpha1/machine_types.go#L42)

### Boostrapping

To minimize code duplication and maximize flexibility, bootstrap clusters with an external Cluster Management API Stack. A Cluster Management API Stack contains all the components needed to provide Kubernetes Cluster Management API for a cluster. [Bootstrap Process Design Details](https://docs.google.com/document/d/1CnzIXtitfbO6Y7ZxVWROGO8jr19t0vooDx-YQ7c2nbI/edit?usp=sharing).

### A new Machine can be created in a declarative way

A new Machine can be created in a declarative way, specifying versions of various components such as the kubelet.
It should also be able to specify provider-specific information such as OS image, instance type, disk configuration, etc., though this will not be portable.

When a cluster is first created with a cluster config file, there is no control plane node or api server. So the user will need to bootstrap a cluster. While the implementation details are specific to the provider, the following guidance should help you:

* Your tool should spin up the external apiserver and the machine controller.
* POST the objects to the apiserver.
* The machine controller creates resources (Machines etc)
* Pivot the apiserver and the machine controller in to the cluster.

#### Configurable Machine Setup

While not mandatory, it is suggested for new providers to support configurable machine setups for creating new machines.
This is to allow flexibility in what startup scripts are used and what versions are supported instead of hardcoding startup scripts into the machine controller.
You can find an example implementation for GCE [here](https://github.com/kubernetes-sigs/cluster-api-provider-gcp/blob/ee60efd89c4d0129a6d42b40d069c0b41d2c4987/cloud/google/machinesetup/config_types.go).

##### GCE Implementation

For GCE, a [config map](https://github.com/kubernetes-sigs/cluster-api-provider-gcp/blob/c0ac09e86b6630bd65c277120883719e514cfdf5/clusterctl/examples/google/provider-components.yaml.template#L151) holds the list of valid machine setup configs,
and the yaml file is volume mounted into the machine controller using a ConfigMap named `machine-setup`.

A [config type](https://github.com/kubernetes-sigs/cluster-api-provider-gcp/blob/ee60efd89c4d0129a6d42b40d069c0b41d2c4987/cloud/google/machinesetup/config_types.go#L70) defines a set of parameters that can be taken from the machine object being created, and maps those parameters to startup scripts and other relevant information.
In GCE, the OS, machine roles, and version info are the parameters that map to a GCP image path and metadata (which contains the startup script).

When creating a new machine, there should be a check for whether the machine setup is supported.
This is done by looking through the valid configs parsed out of the yaml for a config with matching parameters.
If a match is found, then the machine can be created with the startup script found in the config.
If no match is found, then the given machine configuration is not supported.
Getting the script onto the machine and running it on startup is a provider specific implementation detail.

More details can be found in the [design doc](https://docs.google.com/document/d/1OfykBDOXP_t6QEtiYBA-Ax7nSpqohFofyX-wOxrQrnw/edit?ts=5ae11208#heading=h.xgjl2srtytjt), but note that it is GCE specific.

### A specific Machine can be deleted, freeing external resources associated with it.

When the client deletes a Machine object, your controller's reconciler should trigger the deletion of the Machine that backs that machine. The delete is provider specific, but usually requires deleting the VM and freeing up any external resources (like IP).

### A specific Machine can be upgraded or downgraded

These include:

*   A specific Machine can have its kubelet version upgraded or downgraded.
*   A specific Machine can have its OS image upgraded or downgraded.

A sample implementation for an upgrader is [provided here](https://github.com/kubernetes-sigs/cluster-api/blob/master/tools/upgrader/util/upgrade.go). Each machine is upgraded serially, which can amount to:

```
for machine in machines:
    upgrade machine
```

The specific upgrade logic will be implement as part of the machine controller, and is specific to the provider. The user provided provider config will be in `machine.Spec.ProviderSpec`.

Discussion around in-place vs replace upgrades [is here](https://github.com/kubernetes/enhancements/blob/master/keps/sig-cluster-lifecycle/0003-cluster-api.md#in-place-vs-replace).

## Support Channels

Whether you are a user or contributor, official support channels include:

- GitHub issues: https://github.com/kubernetes-sigs/cluster-api/issues/new
- Slack: Chat with us on [Slack](http://slack.k8s.io/): #cluster-api
- Email: [kubernetes-sig-cluster-lifecycle](https://groups.google.com/forum/#!forum/kubernetes-sig-cluster-lifecycle) mailing list

Before opening a new issue or submitting a new pull request, it's helpful to search the project - it's likely that another user has already reported the issue you're facing, or it's a known issue that we're already aware of.
