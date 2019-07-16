# This file creates an AWS Parameter Store config variable for use with the
# "runtimevar/awsparamstore" driver.

locals {
  # The Go CDK URL for the variable: https://gocloud.dev/howto/runtimevar/#awsps.
  awsparamstore_url = "awsparamstore://${aws_ssm_parameter.config.id}?region=${local.aws_region}&decoder=string"
}

# The config variable, with a default value.
resource "aws_ssm_parameter" "config" {
  name      = local.gocdk_random_name
  type      = "String"
  value     = "initial value of AWS Parameter Store config variable"
  overwrite = "true"
}

