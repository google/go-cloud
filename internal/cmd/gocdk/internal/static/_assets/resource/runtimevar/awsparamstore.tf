# TODO(rvangent): Add comments explaining.

locals {
  awsparamstore_url = "awsparamstore://${aws_ssm_parameter.config.id}?region=${local.aws_region}&decoder=string"
}

resource "random_string" "awsparamstore_suffix" {
  special = false
  upper   = false
  length  = 7
}

resource "aws_ssm_parameter" "config" {
  name      = "gocdk-${random_string.awsparamstore_suffix.result}"
  type      = "String"
  value     = "initial value of AWS Parameter Store config variable"
  overwrite = "true"
}
