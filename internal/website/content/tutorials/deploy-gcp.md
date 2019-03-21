---
title: "Tutorial: Deploy to GCP"
linkTitle: "Deploy to GCP"
date: 2019-03-20T09:47:11-07:00
weight: 3
---

In this tutorial, we will deploy an existing Go CDK application called
Guestbook to Google Cloud Platform (GCP).

<!--more-->

{{< snippet "tutorials/deploy/guestbook.md" >}}

## Prerequisites

You will need to install the following software for this tutorial:

-   [Git](https://git-scm.com/)
-   [Go](https://golang.org/doc/install)
-   [Wire](https://github.com/google/wire/blob/master/README.md#installing)
-   [Terraform](https://www.terraform.io/intro/getting-started/install.html)
-   [gcloud CLI](https://cloud.google.com/sdk/downloads)

{{< snippet "/tutorials/deploy/local.md" >}}

## Running on Google Cloud Platform (GCP)

If you want to run this sample on GCP, you need to create a project, download
the gcloud SDK, install `kubectl` and log in.

```shell
# Install kubectl.
gcloud components install kubectl

# Opens a browser to log you into GCP.
gcloud auth login
```

You can then use [Terraform][], a tool for initializing cloud resources, to
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

[Terraform]: https://www.terraform.io/

## Cleanup

To clean up the created resources, run `terraform destroy` inside the `gcp`
directory using the same variables you entered during `terraform apply`.

{{< snippet "/tutorials/deploy/gophers.md" >}}
