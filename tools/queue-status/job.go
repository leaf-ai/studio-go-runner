// Copyright 2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/davecgh/go-spew/spew"

	"github.com/go-stack/stack"

	"github.com/jjeffery/kv"
)

func addGroup(group string, instanceType string, groups map[string][]string) {
	if aGroup, isPresent := groups[group]; isPresent {
		groups[group] = append(aGroup, instanceType)
		return
	}
	groups[group] = []string{instanceType}
	return
}

func jobGenerate(ctx context.Context, cfg *Config, cluster string, template string, queues *Queues) (err kv.Error) {

	sess, err := NewSession(ctx, cfg)
	if err != nil {
		return err
	}

	as := autoscaling.New(sess)

	opts := &autoscaling.DescribeAutoScalingGroupsInput{}

	asGroups := map[string][]string{}
	for {
		groups, errGo := as.DescribeAutoScalingGroups(opts)
		if errGo != nil {
			return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}

		for _, aGroup := range groups.AutoScalingGroups {
			for _, aTag := range aGroup.Tags {
				if aTag.Key == nil || aTag.Value == nil {
					continue
				}
				if strings.HasSuffix(*aTag.Key, "eksctl.io/cluster-name") &&
					*aTag.Value == cluster {
					logger.Trace(*aGroup.AutoScalingGroupName, "stack", stack.Trace().TrimRuntime())
					if aGroup.MixedInstancesPolicy == nil {
						// Get the aGroup.LaunchTemplate and describe it
						if aGroup.LaunchTemplate == nil ||
							aGroup.LaunchTemplate.LaunchTemplateId == nil ||
							len(*aGroup.LaunchTemplate.LaunchTemplateId) == 0 {
							continue
						}
						instances, err := GetInstLT(sess, *aGroup.LaunchTemplate.LaunchTemplateId, *aGroup.LaunchTemplate.Version)

						if err != nil {
							logger.Warn(err.Error(), "stack", stack.Trace().TrimRuntime())
							continue
						}

						for _, aType := range instances {
							addGroup(*aGroup.AutoScalingGroupName, aType, asGroups)
						}
					} else {
						if aGroup.MixedInstancesPolicy.LaunchTemplate == nil ||
							aGroup.MixedInstancesPolicy.LaunchTemplate == nil ||
							aGroup.MixedInstancesPolicy.LaunchTemplate.Overrides == nil {
							continue
						} else {
							for _, override := range aGroup.MixedInstancesPolicy.LaunchTemplate.Overrides {
								if override.InstanceType != nil {
									addGroup(*aGroup.AutoScalingGroupName, *override.InstanceType, asGroups)
								}
							}
						}
					}
				}
			}
		}
		if groups.NextToken == nil || len(*groups.NextToken) == 0 {
			break
		}
		opts.NextToken = groups.NextToken
	}

	fmt.Println(spew.Sdump(asGroups))

	// Locate the ASGs using the cluster name and tags then place these into the Queues
	// Check the ASGs for the number of waiting tasks cluster ordered by the queues
	// Produce new job specifications to fill any gaps
	return nil
}

func GetInstLT(sess *session.Session, templateId string, version string) (instanceTypes []string, err kv.Error) {
	svc := ec2.New(sess)

	input := &ec2.DescribeLaunchTemplateVersionsInput{
		LaunchTemplateId: aws.String(templateId),
	}

	for {
		lts, errGo := svc.DescribeLaunchTemplateVersions(input)
		if errGo != nil {
			return instanceTypes, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}

		for _, lt := range lts.LaunchTemplateVersions {
			if lt.DefaultVersion == nil || !*lt.DefaultVersion {
				continue
			}
			if lt.LaunchTemplateData.InstanceType != nil {
				instanceTypes = append(instanceTypes, *lt.LaunchTemplateData.InstanceType)
			}
		}

		if lts.NextToken == nil || len(*lts.NextToken) == 0 {
			break
		}
		input.NextToken = lts.NextToken
	}
	return instanceTypes, nil
}
