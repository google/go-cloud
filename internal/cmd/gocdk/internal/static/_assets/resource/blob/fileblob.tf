# TODO(rvangent): Add comments explaining.

locals {
  fileblob_bucket_path = "${path.module}/fileblob_scratchdir"
  fileblob_bucket_url  = "file://${local.fileblob_bucket_path}"
}

resource "local_file" "samplefile" {
  content  = "Hello world!"
  filename = "${local.fileblob_bucket_path}/samplefile.txt"
}
