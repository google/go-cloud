// Copyright 2019 The Go Cloud Development Kit Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package launcher

import (
	"context"
	"io/ioutil"
	"log"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ecs"
	cdkaws "gocloud.dev/aws"
	"gocloud.dev/internal/cmd/gocdk/internal/docker"
	"golang.org/x/xerrors"
)

// ECS pushes Docker containers to Amazon Elastic Container Registry (ECR) and
// creates/updates an Elastic Container Service (ECS) task.
type ECS struct {
	Logger         *log.Logger
	ConfigProvider client.ConfigProvider
	DockerClient   *docker.Client
}

// Launch implements Launcher.Launch.
func (l *ECS) Launch(ctx context.Context, input *Input) (*url.URL, error) {
	cluster := specifierStringValue(input.Specifier, "cluster")
	region := specifierStringValue(input.Specifier, "region")
	if cluster == "" || region == "" {
		return nil, xerrors.New("ECS launch: launch_specifier missing cluster and/or region")
	}
	config := &cdkaws.ConfigOverrider{
		Base: l.ConfigProvider,
		Configs: []*aws.Config{{
			Region: aws.String(region),
		}},
	}

	// Push to ECS.
	imageRef, err := l.tagForECS(ctx, input.DockerImage, input.Specifier)
	if err != nil {
		return nil, xerrors.Errorf("ECS launch: %w", err)
	}
	// TODO(light): Run `docker login` with credentials from ECR API.
	// TODO(light): Send docker push output somewhere.
	if err := l.DockerClient.Push(ctx, imageRef, ioutil.Discard); err != nil {
		return nil, xerrors.Errorf("ECS launch: %w", err)
	}

	// Launch on ECS.
	taskDef, err := buildECSTaskDefinition(imageRef, input.Env)
	if err != nil {
		return nil, xerrors.Errorf("ECS launch: %w", err)
	}
	ecsClient := ecs.New(config)
	taskDefOutput, err := ecsClient.RegisterTaskDefinition(taskDef)
	if err != nil {
		return nil, xerrors.Errorf("ECS launch: %w", err)
	}
	runTaskOutput, err := ecsClient.RunTask(&ecs.RunTaskInput{
		Cluster:        aws.String(cluster),
		Count:          aws.Int64(1),
		LaunchType:     aws.String("EC2"),
		TaskDefinition: aws.String(aws.StringValue(taskDefOutput.TaskDefinition.TaskDefinitionArn)),
	})
	if err != nil {
		return nil, xerrors.Errorf("ECS launch: %w", err)
	}
	if len(runTaskOutput.Tasks) == 0 {
		return nil, xerrors.Errorf("ECS launch: could not create task. Your cluster may be at capacity, try stopping old tasks.")
	}
	if len(runTaskOutput.Tasks) > 1 {
		return nil, xerrors.Errorf("ECS launch: found %d tasks (sent 1)", len(runTaskOutput.Tasks))
	}
	task := runTaskOutput.Tasks[0]
	ec2Client := ec2.New(config)
	publicIP, err := l.instanceIP(ctx, ecsClient, ec2Client, cluster, aws.StringValue(task.ContainerInstanceArn))
	if err != nil {
		return nil, xerrors.Errorf("ECS launch: %w", err)
	}
	return &url.URL{
		Scheme: "http",
		Host:   publicIP,
		Path:   "/",
	}, nil
}

// tagForECS tags the given image as needed so that running `docker push`
// will place the image in a registry accessible by ECS. The returned
// string is the image reference that should be passed to ECS.
func (l *ECS) tagForECS(ctx context.Context, imageRef string, launchSpecifier map[string]interface{}) (string, error) {
	rewrittenRef, err := imageRefForECS(imageRef, launchSpecifier)
	if err != nil {
		return "", xerrors.Errorf("docker tag: %w", err)
	}
	if rewrittenRef == imageRef {
		return rewrittenRef, nil
	}
	if err := l.DockerClient.Tag(ctx, imageRef, rewrittenRef); err != nil {
		return "", xerrors.Errorf("docker tag: %w", err)
	}
	return rewrittenRef, nil
}

// imageRefForECS computes the image reference needed to launch the given
// local image reference on ECS and the launch specifier. If the returned
// string is equal to localImage, then no retagging is necessary before pushing.
func imageRefForECS(localImage string, launchSpecifier map[string]interface{}) (string, error) {
	name, tag, digest := docker.ParseImageRef(localImage)
	if tag == ":" {
		return "", xerrors.Errorf("determine image name for ECS: empty tag in %q", localImage)
	}
	if specName := specifierStringValue(launchSpecifier, "image_name"); specName != "" {
		// First, use image name from launch specifier if present.
		if !isECRName(specName) {
			return "", xerrors.Errorf("determine image name for ECS: launch specifier image_name = %q, not an ECR name", specName)
		}
		name = specName
	} else if !isECRName(name) {
		// Error if the image name is not an ECR name.
		// We can't infer account number.
		return "", xerrors.New("determine image name for ECS: launch specifier image_name empty")
	}
	return name + tag + digest, nil
}

// buildECSTaskDefinition generates an ECS task definition for the given
func buildECSTaskDefinition(imageRef string, env []string) (*ecs.RegisterTaskDefinitionInput, error) {
	family := ecsFamilyName(imageRef)
	var envKV []*ecs.KeyValuePair
	for i, inputVar := range env {
		eqIdx := strings.IndexByte(inputVar, '=')
		if eqIdx == -1 {
			return nil, xerrors.Errorf("environment variables should be in the form VARNAME=VALUE, but env[%d] = %q", i, inputVar)
		}
		envKV = append(envKV, &ecs.KeyValuePair{
			Name:  aws.String(inputVar[:eqIdx]),
			Value: aws.String(inputVar[eqIdx+1:]),
		})
	}
	envKV = append(envKV, &ecs.KeyValuePair{
		Name:  aws.String("PORT"),
		Value: aws.String("8080"),
	})
	return &ecs.RegisterTaskDefinitionInput{
		RequiresCompatibilities: []*string{
			aws.String("EC2"),
		},
		Family: aws.String(family),
		ContainerDefinitions: []*ecs.ContainerDefinition{
			{
				Name:      aws.String(family),
				Image:     aws.String(imageRef),
				Memory:    aws.Int64(500),
				Essential: aws.Bool(true),
				PortMappings: []*ecs.PortMapping{
					{
						ContainerPort: aws.Int64(8080),
						HostPort:      aws.Int64(80),
					},
				},
				Environment: envKV,
			},
		},
	}, nil
}

// instanceIP queries ECS and EC2 to find the public IP address of the given
// ECS container instance.
func (l *ECS) instanceIP(ctx context.Context, ecsClient *ecs.ECS, ec2Client *ec2.EC2, cluster, containerInstanceARN string) (string, error) {
	containerInstanceOutput, err := ecsClient.DescribeContainerInstances(&ecs.DescribeContainerInstancesInput{
		Cluster:            aws.String(cluster),
		ContainerInstances: []*string{aws.String(containerInstanceARN)},
	})
	if err != nil {
		return "", xerrors.Errorf("find ECS container instance IP: %w", err)
	}
	if len(containerInstanceOutput.ContainerInstances) != 1 {
		return "", xerrors.Errorf("find ECS container instance IP: found %d container instance (want 1)", len(containerInstanceOutput.ContainerInstances))
	}
	containerInstance := containerInstanceOutput.ContainerInstances[0]
	ec2InstanceID := aws.StringValue(containerInstance.Ec2InstanceId)
	instanceOutput, err := ec2Client.DescribeInstances(&ec2.DescribeInstancesInput{
		InstanceIds: []*string{aws.String(ec2InstanceID)},
	})
	if err != nil {
		return "", xerrors.Errorf("find ECS container instance IP: %w", err)
	}
	publicIP := aws.StringValue(instanceOutput.Reservations[0].Instances[0].PublicIpAddress)
	return publicIP, nil
}

// ecsFamilyName infers a family/container name given the image name.
func ecsFamilyName(image string) string {
	name, _, _ := docker.ParseImageRef(image)
	if i := strings.LastIndexByte(name, '/'); i != -1 {
		name = name[i+1:]
	}
	if name == "" {
		return "gocdk-launch"
	}
	// TODO(light): Maybe sanitize name?
	return name
}

// isECRName reports whether the given image name or reference identifies an
// image on Elastic Container Registry.
//
// The host name format is documented here:
// https://docs.aws.amazon.com/AmazonECR/latest/userguide/Registries.html
func isECRName(image string) bool {
	registry, _, _ := docker.ParseImageRef(image)
	if i := strings.IndexByte(registry, '/'); i != -1 {
		registry = registry[:i]
	}
	return strings.HasSuffix(registry, ".amazonaws.com") && strings.Contains(registry, ".dkr.ecr.")
}
