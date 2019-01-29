# Copyright 2018 The Go Cloud Development Kit Authors
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

resource "google_storage_bucket" "bucket" {
  name          = "${var.name}"
  project       = "go-cloud-modules"
  location      = "US"
  storage_class = "${var.storage_class}"
}

resource "google_storage_bucket_acl" "bucket" {
  bucket         = "${google_storage_bucket.bucket.name}"
  predefined_acl = "projectPrivate"
}

resource "google_storage_default_object_acl" "bucket" {
  bucket = "${google_storage_bucket.bucket.name}"

  role_entity = [
    "OWNER:project-owners-450909594463",
    "OWNER:project-editors-450909594463",
    "READER:project-viewers-450909594463",
  ]
}

resource "google_storage_bucket_iam_binding" "object_viewer" {
  bucket  = "${google_storage_bucket.bucket.name}"
  role    = "roles/storage.objectViewer"
  members = ["allUsers"]
}

resource "google_storage_bucket_iam_binding" "legacy_reader" {
  bucket  = "${google_storage_bucket.bucket.name}"
  role    = "roles/storage.legacyBucketReader"
  members = ["projectViewer:go-cloud-modules"]
}

resource "google_storage_bucket_iam_binding" "legacy_owner" {
  bucket = "${google_storage_bucket.bucket.name}"
  role   = "roles/storage.legacyBucketOwner"

  members = [
    "projectEditor:go-cloud-modules",
    "projectOwner:go-cloud-modules",
  ]
}
