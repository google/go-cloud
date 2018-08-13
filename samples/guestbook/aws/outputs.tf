# Copyright 2018 The Go Cloud Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

output "region" {
  value       = "${var.region}"
  description = "Region the resources were created in."
}

output "bucket" {
  value       = "${aws_s3_bucket.guestbook.id}"
  description = "Name of the S3 bucket created to store images."
}

output "database_host" {
  value       = "${aws_db_instance.guestbook.address}"
  description = "Host name of the RDS MySQL database."
}

output "database_root_password" {
  value       = "${random_string.db_password.result}"
  sensitive   = true
  description = "Password for the root user of the RDS MySQL databse."
}

output "paramstore_var" {
  value       = "${var.paramstore_var}"
  description = "Location of the SSM Parameter Store Message of the Day variable."
}

output "instance_host" {
  value       = "${aws_instance.guestbook.public_ip}"
  description = "Address of the EC2 instance."
}
