# This file creates an AWS DynamobDB table for use with the
# "docstore/awsdynamodb" driver.

locals {
  # The Go CDK URL for the collection: https://gocloud.dev/howto/docstore/#dynamodb.
  awsdynamodb_url = "dynamodb://${aws_dynamodb_table.table.name}?region=${local.aws_region}&partition_key=Key"
}

# The DynamoDB table, with a string Key field used as the hash_key.
resource "aws_dynamodb_table" "table" {
  name         = local.gocdk_random_name
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "Key"

  attribute {
    name = "Key"
    type = "S"
  }
}
