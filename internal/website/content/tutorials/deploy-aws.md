---
title: "Tutorial: Deploy to AWS"
linkTitle: "Deploy to AWS"
date: 2019-03-19T19:17:56-07:00
weight: 2
---

In this tutorial, we will deploy an existing Go CDK application called
Guestbook to Amazon Web Services (AWS).

<!--more-->

{{< snippet "tutorials/deploy/guestbook.md" >}}

## Prerequisites

You will need to install the following software for this tutorial:

-   [Git](https://git-scm.com/)
-   [Go](https://golang.org/doc/install)
-   [Wire](https://github.com/google/wire/blob/master/README.md#installing)
-   [Docker Desktop](https://docs.docker.com/install/)
-   [Terraform](https://www.terraform.io/intro/getting-started/install.html)
-   [aws CLI](https://docs.aws.amazon.com/cli/latest/userguide/installing.html)

{{< snippet "tutorials/deploy/local.md" >}}

## Running on Amazon Web Services (AWS)

If you want to run this sample on AWS, you need to set up an account, download
the AWS command line interface, and log in. There's [help here][AWS Config Help]
if you need it.

```shell
aws configure
```

[AWS Config Help]: https://docs.aws.amazon.com/cli/latest/userguide/cli-chap-configure.html

### Agree to the Debian Terms and Conditions

You have to agree to the [Debian Terms and Conditions][] in order to
provision the resources. Click through the "Continue to Subscribe" button at the
top, then log in to your AWS account and subscribe to Debian.

[Debian Terms and Conditions]: https://aws.amazon.com/marketplace/pp?sku=55q52qvgjfpdj2fpfy9mb1lo4

### SSH Key

You will also need an SSH key to SSH into the EC2 instance. If you don't already
have one, you can follow [this guide from GitHub][GitHub SSH]. Follow the
instructions for "Adding your key to the ssh-agent" if you want the key to
persist across terminal sessions.

```shell
ssh-add
```
[GitHub SSH]: https://help.github.com/articles/generating-a-new-ssh-key-and-adding-it-to-the-ssh-agent/

### Provision resources with Terraform

You can now use Terraform, a tool for initializing cloud resources, to set up
your project. This will create an EC2 instance you can SSH to and run your
binary.

```shell
# Build for deploying on the AWS Linux VM.
GOOS=linux GOARCH=amd64 go build

# Enter AWS directory from samples/guestbook.
cd aws
terraform init

# Provisioning can take up to 10 minutes.
# Keep track of the output of this command as it is needed later.
# You can replace us-west-1 with whatever region you want.
terraform apply -var region=us-west-1 -var ssh_public_key="$(cat ~/.ssh/id_rsa.pub)"
```

### Connect to the new server and run the guestbook binary

You now need to connect to the new remote server to execute the `guestbook`
binary. The final output of `terraform apply` lists the variables `guestbook`
requires as arguments. Here's an example, with actual strings replaced with
`[redacted]`:

```shell
# Output from "terraform apply" command....
<snip>

Outputs:

bucket = [redacted]
database_host = [redacted]
database_root_password = <sensitive>
instance_host = [redacted]
paramstore_var = /guestbook/motd
region = us-west-1

# Print out the database root password, since we'll need it below
# Terraform hides it by default in the Outputs above.
localhost$ terraform output database_root_password
[redacted]

# SSH into the EC2 instance.
localhost$ ssh "admin@$( terraform output instance_host )"

# Fill in each command-line argument with the values from the
# Terraform output above.
server$ AWS_REGION=<your region> ./guestbook \
  -env=aws \
  -bucket=<your bucket> \
  -db_host=<your database_host> \
  -db_user=root \
  -db_password=<your database_root_password> \
  -motd_var=/guestbook/motd
```

### View the guestbook application

You can now visit the server at `http://<your instance_host>:8080/`.

## Cleanup

To clean up the created resources, run `terraform destroy` inside the `aws`
directory using the same variables you entered during `terraform apply`.

{{< snippet "/tutorials/deploy/gophers.md" >}}
