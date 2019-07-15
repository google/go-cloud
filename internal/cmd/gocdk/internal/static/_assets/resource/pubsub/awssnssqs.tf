# This file creates an AWS SNS topic and a SQS queue subscribed to it, for use
# with the "pubsub/awssnssqs" driver.

locals {
  # The Go CDK URL for the topic: https://gocloud.dev/howto/pubsub/publish#sns.
  awssns_topic_url = "awssns://${aws_sns_topic.topic.arn}?region=${local.aws_region}"
  # The Go CDK URL for the subscription: https://gocloud.dev/howto/pubsub/subscribe#sqs.
  awssqs_subscription_url = "awssqs://${replace(aws_sqs_queue.queue.id, "https://", "")}?region=${local.aws_region}"
}

# The SNS topic.
resource "aws_sns_topic" "topic" {
  name_prefix = "gocdk-"
}

# The SQS queue.
resource "aws_sqs_queue" "queue" {
  name_prefix = "gocdk-"
}

# Subscribe the queue to the topic.
resource "aws_sns_topic_subscription" "topic_to_queue" {
  topic_arn = aws_sns_topic.topic.arn
  protocol  = "sqs"
  endpoint  = aws_sqs_queue.queue.arn
}

# Add a policy to the queue that allows the SNS topic to send messages to it.
resource "aws_sqs_queue_policy" "queue_policy" {
  queue_url = aws_sqs_queue.queue.id

  policy = <<POLICY
{
  "Version": "2012-10-17",
  "Id": "AllowQueue",
  "Statement": [
    {
      "Sid": "First",
      "Effect": "Allow",
      "Principal": "*",
      "Action": "SQS:SendMessage",
      "Resource": "${aws_sqs_queue.queue.arn}",
      "Condition": {
        "ArnEquals": {
          "aws:SourceArn": "${aws_sns_topic.topic.arn}"
        }
      }
    }
  ]
}
POLICY

}

