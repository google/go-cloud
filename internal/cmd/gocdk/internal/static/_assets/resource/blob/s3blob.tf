# This file creates an S3 bucket for use with the "blob/s3blob" driver.

locals {
  # The Go CDK URL for the bucket: https://gocloud.dev/howto/blob/#s3.
  s3blob_bucket_url = "s3://${aws_s3_bucket.bucket.id}?region=${aws_s3_bucket.bucket.region}"
}

# The actual bucket.
resource "aws_s3_bucket" "bucket" {
  bucket_prefix = "gocdk-"
  force_destroy = true
}

