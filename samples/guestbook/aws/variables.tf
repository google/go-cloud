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

variable "region" {
  type        = "string"
  description = "Region to create resources in. See https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/Concepts.RegionsAndAvailabilityZones.html for valid values."
}

variable "ssh_public_key" {
  type        = "string"
  description = "A public key line in .ssh/authorized_keys format to use to authenticate to your instance. This must be added to your SSH agent for provisioning to succeed."
}

variable "paramstore_var" {
  default     = "/guestbook/motd"
  description = "The location in SSM Parameter Store of the Message of the Day variable."
}
