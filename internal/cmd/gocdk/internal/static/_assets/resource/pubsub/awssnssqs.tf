# TODO(rvangent): Add comments explaining.

locals {
  awssns_topic_url        = "awssns://${aws_sns_topic.topic.arn}?region=${local.aws_region}"
  awssqs_subscription_url = "awssqs://${replace(aws_sqs_queue.queue.id, "https://", "")}?region=${local.aws_region}"
}

resource "aws_sns_topic" "topic" {
  name_prefix = "gocdk-"
}

resource "aws_sqs_queue" "queue" {
  name_prefix = "gocdk-"
}

resource "aws_sns_topic_subscription" "topic_to_queue" {
  topic_arn = aws_sns_topic.topic.arn
  protocol  = "sqs"
  endpoint  = aws_sqs_queue.queue.arn
}

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

