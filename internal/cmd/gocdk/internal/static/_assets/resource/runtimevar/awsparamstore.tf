# TODO(rvangent): Add comments explaining.

locals {
  awsparamstore_url = "awsparamstore://${aws_ssm_parameter.config.id}?region=${local.aws_region}&decoder=string"
}

resource "aws_ssm_parameter" "config" {
  name      = local.gocdk_random_name
  type      = "String"
  value     = "initial value of AWS Parameter Store config variable"
  overwrite = "true"
}

