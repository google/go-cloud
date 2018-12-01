# Guestbook Sample

Guestbook is a sample application that records visitors' messages, displays a
cloud banner, and an administrative message. The main business logic is
written in a cloud-agnostic manner using MySQL, the generic blob API, and the
generic runtimevar API. All platform-specific code is set up by 
[Wire](https://github.com/google/wire).

## Prerequisites

You will need to install the following software to run this sample:

- [Go](https://golang.org/doc/install)
- [Wire](https://github.com/google/wire/blob/master/README.md#installing)
- [Docker](https://docs.docker.com/install/)
- [Terraform][TF]
- [jq](https://stedolan.github.io/jq/download/)
- [gcloud CLI](https://cloud.google.com/sdk/downloads), if you want to use GCP
- [aws CLI](https://docs.aws.amazon.com/cli/latest/userguide/installing.html),
  if you want to use AWS

## Building

Run the following in the `samples/guestbook` directory:

```shell
go generate && go build
```

## Running Locally

You will need to run a local MySQL database server, set up a directory that
simulates a bucket, and create a local message of the day. `localdb/main.go` is a
program that runs a temporary database using Docker:

```shell
go get ./localdb/... # Get package dependencies.
go run localdb/main.go
```

In another terminal, you can run:

```shell
# Set a local Message of the Day
echo 'Hello, World!' > motd.txt

# Run the server.
./guestbook -env=local -bucket=blobs -motd_var=motd.txt
```

Your server should be running on http://localhost:8080/.

You can stop the MySQL database server with Ctrl-\\. MySQL ignores Ctrl-C
(SIGINT).

## Running on Google Cloud Platform (GCP)

If you want to run this sample on GCP, you need to create a project, download
the gcloud SDK, install `kubectl` and log in.

``` shell
# Install kubectl.
gcloud components install kubectl

# Opens a browser to log you into GCP.
gcloud auth login
```

You can then use [Terraform][TF], a tool for initializing cloud resources, to
set up your project. Finally, this sample provides a script for building the
Guestbook binary and deploying it to the Kubernetes cluster created by
Terraform.

```shell
gcloud auth application-default login
cd gcp
terraform init

# Terraform will prompt you for your GCP project ID, desired region,
# and desired zone.
terraform apply

go run deploy/main.go
```

The deploy script will display the URL of your running service.

To clean up the created resources, run `terraform destroy` inside the `gcp`
directory using the same variables you entered during `terraform apply`.

## Running on Amazon Web Services (AWS)

If you want to run this sample on AWS, you need to set up an account, download
the AWS command line interface, and log in. You will also need an SSH key. If you
don't already have one, you can follow [this guide from GitHub][GitHub SSH]. Follow the instructions for "Adding your key to the ssh-agent" if you want the key to persist across terminal sessions.

### Agree to the AWS Terms and Conditions
You have to agree to the [AWS Terms and Conditions][AWS T&C] in order to provision the resources.

### Provision resources with Terraform
With the SSH keys generated and AWS Terms and Conditions signed, you can then
use Terraform, a tool for initializing cloud resources, to set up your project.
This will create an EC2 instance you can SSH to and run your binary.

```shell
aws configure
ssh-add

# Build for deploying on the AWS Linux VM.
GOOS=linux GOARCH=amd64 go build

# Enter AWS directory from samples/guestbook.
cd aws
terraform init

# Provisioning can take up to 10 minutes.
# Keep track of the output of this command as it is needed later.
terraform apply -var region=us-west-1 -var ssh_public_key="$(cat ~/.ssh/id_rsa.pub)"
```

### Connect to the new server and run the guestbook binary
You now need to connect to the new remote server to execute the `guestbook` binary. The final output of `terraform apply` lists the required variables `guestbook` requires to execute. Here's an example (with redacted output!):

```shell
localhost$ terraform apply

<snip>

Outputs:

bucket = guestbook[timestamp]
database_host = guestbook[timestamp][redacted].us-west-1.rds.amazonaws.com
database_root_password = <sensitive>
instance_host = [redacted]
paramstore_var = /guestbook/motd
region = us-west-1

# SSH into the EC2 instance.
localhost$ ssh "admin@$( terraform output instance_host )"

server$ AWS_REGION=us-west-1 ./guestbook -env=aws \
  -bucket=guestbook[timestamp] \
	-db_host=guestbook[timestamp][redacted].us-west-1.rds.amazonaws.com \
	-motd_var=/guestbook/motd
```

### View the guestbook application
You can then visit the server at `http://INSTANCE_HOST:8080/`, where
`INSTANCE_HOST` is the value of `terraform output instance_host` run on your
local machine.

### Cleanup
To clean up the created resources, run `terraform destroy` inside the `aws`
directory using the same variables you entered during `terraform apply`.

[GitHub SSH]: https://help.github.com/articles/generating-a-new-ssh-key-and-adding-it-to-the-ssh-agent/
[AWS T&C]: https://aws.amazon.com/marketplace/pp?sku=55q52qvgjfpdj2fpfy9mb1lo4

## Gophers

The Go gopher was designed by Renee French and used under the [Creative Commons
3.0 Attributions](https://creativecommons.org/licenses/by/3.0/) license.

[TF]: https://www.terraform.io/intro/getting-started/install.html
