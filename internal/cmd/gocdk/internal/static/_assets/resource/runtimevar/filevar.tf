# This file creates a local file for use with the "runtimevar/filevar" driver.

locals {
  # The path to the local directory to create.
  filevar_dir_path = "${path.cwd}/filevar_scratchdir"
  # The Go CDK URL for the variable: https://gocloud.dev/howto/runtimevar/#local.
  filevar_url = "file://${local_file.configfile.filename}?decoder=string"
}

# Terraform doesn't support creating an empty directory directly, so create a
# file in the target directory.
resource "local_file" "configfile" {
  content  = "config value"
  filename = "${local.filevar_dir_path}/config.txt"
}

