---
title: "Tutorial: Connect to Azure"
linkTitle: "Connect to Azure"
date: 2019-03-20T09:59:16-07:00
weight: 4
---

In this tutorial, we will run an existing Go CDK application called Guestbook
backed by services running on Microsoft Azure. The Go CDK doesn't have
support for SQL on Azure yet ([#1305][]), so we'll run MySQL and the
binary locally. This tutorial will show how to use Azure storage for the MOTD
and Gopher logo.

[#1305]: https://github.com/google/go-cloud/issues/1305

<!--more-->

{{< snippet "/tutorials/deploy/guestbook.md" >}}

## Prerequisites

You will need to install the following software for this tutorial:

-   [Git](https://git-scm.com/)
-   [Go](https://golang.org/doc/install)
-   [Wire](https://github.com/google/wire/blob/master/README.md#installing)
-   [Docker Desktop](https://docs.docker.com/install/)
-   [Terraform](https://www.terraform.io/intro/getting-started/install.html)
-   [az CLI](https://docs.microsoft.com/en-us/cli/azure/install-azure-cli?view=azure-cli-latest)

{{< snippet "/tutorials/deploy/local.md" >}}

## Running on Azure

If you want to run this sample on Azure, you first need to set up an Azure
account. Use the `az` CLI to log in.

```shell
az login
```

### Provision resources with Terraform

We'll use [Terraform][], a tool for initializing cloud resources, to set up your
project.

```shell
# Enter the Azure directory from samples/guestbook.
cd azure
terraform init

# Provisioning can take up to 10 minutes.
# Keep track of the output of this command as it is needed later.
terraform apply -var location="West US"

<snip>
Outputs:

access_key = [redacted]
storage_account = [redacated]
storage_container = [redacted]
```

[Terraform]: https://www.terraform.io/

### Running

You will need to run a local MySQL database server, similar to what we did for
running locally earlier. Open a new terminal window, and run:

```shell
cd .. # back up to samples/guestbook
go get ./localdb/... # Get package dependencies.
go run localdb/main.go
```

In the original terminal, add your Azure credentials to the environment and run
the `guestbook` application:

```shell
# You should be in the "samples/guestbook/azure" directory.

# Enter the storage_account from the Terraform output earlier.
export AZURE_STORAGE_ACCOUNT=<your storage_account>
# Enter the access_key from the Terraform output earlier.
export AZURE_STORAGE_KEY=<your access_key>

# Run the binary.
# Fill in the -bucket command-line argument with the value from the Terraform
# output.
#
./guestbook -env=azure -bucket=<your storage_container> -motd_var=motd
```

Your server is now running on http://localhost:8080/.

You can stop the MySQL database server with Ctrl-\\. MySQL ignores Ctrl-C
(SIGINT).

## Cleanup

To clean up the created resources, run `terraform destroy` inside the `azure`
directory using the same variables you entered during `terraform apply`.

{{< snippet "/tutorials/deploy/gophers.md" >}}
