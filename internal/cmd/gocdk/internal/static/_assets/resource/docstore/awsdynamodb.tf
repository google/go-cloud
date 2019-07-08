# TODO(rvangent): Add comments explaining.

locals {
  awsdynamodb_url = "dynamodb://${aws_dynamodb_table.table.name}?region=${local.aws_region}&partition_key=Key&sort_key=Key"
}

resource "aws_dynamodb_table" "table" {
  name         = local.gocdk_random_name
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "Key"

  attribute {
    name = "Key"
    type = "S"
  }
}
