# TODO(rvangent): Add comments explaining.

locals {
  blob_bucket_path    = "${path.cwd}/fileblob_scratchdir"
  fileblob_bucket_url = "file://${local.blob_bucket_path}"
}

resource "local_file" "samplefile" {
  content  = "Hello world!"
  filename = "${local.blob_bucket_path}/samplefile.txt"
}
