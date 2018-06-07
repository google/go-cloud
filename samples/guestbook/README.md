# Guestbook Sample

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

```shell
# First time, for gowire.
$ vgo vendor

# Now build:
$ gowire && vgo build
```

## Running Locally

```shell
./start-localdb.sh
./guestbook -env=local
```

Stop the MySQL database server with:

```shell
$ docker stop guestbook-sql
```

## Running on Google Cloud Platform (GCP)

```shell
gcloud auth application-default login
gcloud config set project [MYPROJECT]
export GCLOUD_REGION=us-central1
export GCLOUD_ZONE=us-central1-a
cd gcp
terraform apply
```

Modify `inject_gcp.go` with the values from `terraform output`, then run:

```shell
gowire ..
vgo build -o ../guestbook ..
./deploy.sh
```

To clean up the created resources, run `terraform destroy` inside the `gcp`
directory.

## Running on Amazon Web Services (AWS)

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
