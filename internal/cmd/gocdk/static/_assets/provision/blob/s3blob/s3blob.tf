# TODO(rvangent): Add comments explaining.

locals {
  s3blob_bucket_url = "s3://${aws_s3_bucket.bucket.id}?region=${var.aws_region}"
}

resource "aws_s3_bucket" "bucket" {
  bucket_prefix = "gocdk-"
}
