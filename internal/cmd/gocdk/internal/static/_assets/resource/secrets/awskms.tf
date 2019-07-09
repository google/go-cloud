# TODO(rvangent): Add comments explaining.

locals {
  awskms_url = "awskms://${aws_kms_key.key.arn}?region=${local.aws_region}"
}

resource "aws_kms_key" "key" {
  description = local.gocdk_random_name
}
