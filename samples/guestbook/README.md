# Guestbook Sample

Guestbook is a sample application that records visitors' messages, displays a
cloud banner, and an administrative message. The main business logic is
written in a cloud-agnostic manner using MySQL, the generic blob API, and the
generic runtimevar API. Each of the platform-specific code is set up by Wire.

## Prerequisites

You will need to install the following software to run this sample:

- [Go](https://golang.org/doc/install) and
  [vgo](https://go.googlesource.com/vgo)
- [Docker](https://docs.docker.com/install/)
- [Terraform](https://www.terraform.io/intro/getting-started/install.html)
- [jq](https://stedolan.github.io/jq/download/)
- [gcloud CLI](https://cloud.google.com/sdk/downloads), if you want to use GCP
- [aws CLI](https://docs.aws.amazon.com/cli/latest/userguide/installing.html),
  if you want to use AWS

## Building

`gowire` is not compatible with `vgo` yet, so you must run `vgo vendor`
first to download all the dependencies in `go.mod`. Running `gowire`
generates the Wire code.

```shell
# First time, for gowire.
$ vgo vendor

# Now build:
$ gowire && vgo build
```

## Running Locally

You will need to run a local MySQL database server using Docker, and then you
can run the server.

```shell
./start-localdb.sh
./guestbook -env=local
```

Stop the MySQL database server with:

```shell
$ docker stop guestbook-sql
```

## Running on Google Cloud Platform (GCP)

If you want to run this sample on GCP, you need to create a project, download
the gcloud SDK, and log in. You can then use Terraform, a tool for
initializing cloud resources, to set up your project.

```shell
gcloud auth application-default login
gcloud config set project [MYPROJECT]
export GCLOUD_REGION=us-central1
export GCLOUD_ZONE=us-central1-a
cd gcp
terraform apply
```

Modify `inject_gcp.go` with the values from `terraform output`, then run the
following to rebuild the server:

```shell
gowire ..
vgo build -o ../guestbook ..
./deploy.sh
```

To clean up the created resources, run `terraform destroy` inside the `gcp`
directory.

## Running on Amazon Web Services (AWS)

If you want to run this sample on AWS, you need to set up an account, download
the AWS command line interface, and log in. You can then use Terraform, a tool
for initializing cloud resources, to set up your project.

```shell
aws configure
vgo build
cd aws
terraform apply
```

Modify `inject_aws.go` with the values from `terraform output`, then run:

```shell
# Rebuild the server.
gowire ..
vgo build -o ../guestbook ..

# Copy the server to an EC2 instance.
terraform taint aws_instance.guestbook
terraform apply

# SSH into the EC2 instance and run the server.
ssh "admin@$( terraform output instance_host )"
AWS_REGION=us-west-1 ./guestbook -env=aws
```

To clean up the created resources, run `terraform destroy` inside the `aws`
directory.
