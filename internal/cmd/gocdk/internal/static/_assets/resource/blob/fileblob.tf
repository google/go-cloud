# This file creates a local directory on disk for use with the "blob/fileblob"
# driver.

locals {
  # The path to the local directory to create.
  fileblob_bucket_path = "${path.cwd}/fileblob_scratchdir"
  # The Go CDK URL for the bucket: https://gocloud.dev/howto/blob/#local.
  fileblob_bucket_url = "file://${local.fileblob_bucket_path}"
}

# Terraform doesn't support creating an empty directory directly, so create a
# file in the target directory.
resource "local_file" "samplefile" {
  content  = "Hello world!"
  filename = "${local.fileblob_bucket_path}/samplefile.txt"
}

