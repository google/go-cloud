# The Go CDK Project Terraform Setup

This is a [Terraform][] configuration for the Go CDK open source project. It
manages GitHub ACLs, issue labels, and the module proxy buckets on GCS. To apply
the configuration to the project's resources, [sign into the gcloud
CLI][gcloud auth login], grab a [GitHub access token][], and then do the
following:

```bash
internal/admin$ echo 'github_token = "INSERT TOKEN HERE"' > terraform.tfvars
internal/admin$ terraform init
internal/admin$ terraform apply
```

[gcloud auth login]: https://cloud.google.com/sdk/docs/authorizing#running_gcloud_auth_login
[GitHub access token]: https://github.com/settings/tokens/new?scopes=repo
[Terraform]: https://www.terraform.io/
