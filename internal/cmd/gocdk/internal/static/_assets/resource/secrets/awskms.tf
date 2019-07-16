# This file creates an AWS KMS key for use with the "secrets/awskms" driver.

locals {
  # The Go CDK URL for the key: https://gocloud.dev/howto/secrets/#aws.
  awskms_url = "awskms://${aws_kms_key.key.arn}?region=${local.aws_region}"
}

# The KMS key.
resource "aws_kms_key" "key" {
  description = local.gocdk_random_name
}
