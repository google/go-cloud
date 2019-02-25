# Guestbook Sample

Guestbook is a sample application that records visitors' messages, displays a
cloud banner, and an administrative message. The main business logic is written
in a cloud-agnostic manner using MySQL, the generic blob API, and the generic
runtimevar API. All platform-specific code is set up by
[Wire](https://github.com/google/wire).

## Prerequisites

You will need to install the following software to run this sample:

-   [Go](https://golang.org/doc/install)
-   [Wire](https://github.com/google/wire/blob/master/README.md#installing)
-   [Docker Desktop](https://docs.docker.com/install/)

To run the sample on a Cloud provider (GCP, AWS, or Azure), you will also need:

-   [Terraform][TF]
-   [gcloud CLI](https://cloud.google.com/sdk/downloads), if you want to use GCP
-   [aws CLI](https://docs.aws.amazon.com/cli/latest/userguide/installing.html),
    if you want to use AWS
-   [az CLI](https://docs.microsoft.com/en-us/cli/azure/install-azure-cli?view=azure-cli-latest),
    if you want to use Azure

## Building

Run the following in the `samples/guestbook` directory:

```shell
go generate && go build
```

## Running Locally

You will need to run a local MySQL database server and create a local message of
the day. `localdb/main.go` runs the local MySQL database server using Docker:

```shell
go get ./localdb/... # Get package dependencies.
go run localdb/main.go
```

In another terminal, run the `guestbook` application:

```shell
# Set a local Message of the Day.
echo 'Hello, World!' > motd.txt

# Run the server.
# For blob, it uses fileblob, pointing at the local directory ./blobs.
# For runtimevar, it uses filevar, pointing at the local file ./motd.txt.
#   You can update the ./motd.txt while the server is running, refresh
#   the page, and see it change.
./guestbook -env=local -bucket=blobs -motd_var=motd.txt
```

Your server is now running on http://localhost:8080/.

You can stop the MySQL database server with Ctrl-\\. MySQL ignores Ctrl-C
(SIGINT).

## Running on Google Cloud Platform (GCP)

If you want to run this sample on GCP, you need to create a project, download
the gcloud SDK, install `kubectl` and log in.

```shell
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
the AWS command line interface, and log in. There's [help here][AWS Config Help]
if you need it.

```shell
aws configure
```

### Agree to the AWS Terms and Conditions

You have to agree to the [AWS Terms and Conditions][AWS T&C] in order to
provision the resources. Click through the "Continue to Subscribe" button at the
top, then log in to your AWS account and subscribe to Debian.

### SSH Key

You will also need an SSH key to SSH into the EC2 instance. If you don't already
have one, you can follow [this guide from GitHub][GitHub SSH]. Follow the
instructions for "Adding your key to the ssh-agent" if you want the key to
persist across terminal sessions.

```shell
ssh-add
```

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
server$ AWS_REGION=<your region> ./guestbook -env=aws -bucket=<your bucket> -db_host=<your database_host> -db_user=root -db_password=<your database_root_password> -motd_var=/guestbook/motd
```

### View the guestbook application

You can now visit the server at `http://<your instance_host>:8080/`.

### Cleanup

To clean up the created resources, run `terraform destroy` inside the `aws`
directory using the same variables you entered during `terraform apply`.

[AWS Config Help]: https://docs.aws.amazon.com/cli/latest/userguide/cli-chap-configure.html
[AWS T&C]: https://aws.amazon.com/marketplace/pp?sku=55q52qvgjfpdj2fpfy9mb1lo4
[GitHub SSH]: https://help.github.com/articles/generating-a-new-ssh-key-and-adding-it-to-the-ssh-agent/

## Running on Azure

If you want to run this sample on Azure, you first need to set up an Azure
account. Use the `az` CLI to log in.

```shell
az login
```

The Go CDK doesn't have support for SQL on Azure yet
(https://github.com/google/go-cloud/issues/1305), so we'll run MySQL and the
guestbook binary locally. Guestbook will get the Gopher logo and MOTD from Azure
storage.

### Provision resources with Terraform

We'll use Terraform, a tool for initializing cloud resources, to set up your
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

### Cleanup

To clean up the created resources, run `terraform destroy` inside the `azure`
directory using the same variables you entered during `terraform apply`.

## Gophers

The Go gopher was designed by Renee French and used under the
[Creative Commons 3.0 Attributions](https://creativecommons.org/licenses/by/3.0/)
license.

[TF]: https://www.terraform.io/intro/getting-started/install.html
