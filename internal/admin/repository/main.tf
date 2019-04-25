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

resource "github_repository" "repo" {
  name         = "${var.name}"
  description  = "${var.description}"
  homepage_url = "${var.homepage_url}"
  topics       = "${var.topics}"

  has_downloads = true
  has_issues    = true
  has_projects  = false
  has_wiki      = false

  default_branch     = "master"
  allow_merge_commit = false
  allow_squash_merge = true
  allow_rebase_merge = false
}

resource "github_branch_protection" "default_branch" {
  repository     = "${github_repository.repo.name}"
  branch         = "${github_repository.repo.default_branch}"
  enforce_admins = true

  required_status_checks {
    strict = false

    contexts = [
      "Travis CI - Pull Request",
      "cla/google",
    ]
  }

  required_pull_request_reviews {}
}

# Permissions

data "github_team" "eng" {
  slug = "go-cloud-eng"
}

resource "github_team_repository" "eng" {
  team_id    = "${data.github_team.eng.id}"
  repository = "${github_repository.repo.name}"
  permission = "admin"
}

data "github_team" "devrel" {
  slug = "go-devrel"
}

resource "github_team_repository" "devrel" {
  team_id    = "${data.github_team.devrel.id}"
  repository = "${github_repository.repo.name}"
  permission = "push"
}

resource "github_repository_collaborator" "sameer" {
  repository = "${github_repository.repo.name}"
  username   = "Sajmani"
  permission = "admin"
}

resource "github_repository_collaborator" "rsc" {
  repository = "${github_repository.repo.name}"
  username   = "rsc"
  permission = "push"
}

# Labels

resource "github_issue_label" "blocked" {
  repository  = "${github_repository.repo.name}"
  name        = "blocked"
  color       = "e89884"
  description = "Blocked on a different issue"
}

resource "github_issue_label" "bug" {
  repository  = "${github_repository.repo.name}"
  name        = "bug"
  color       = "d73a4a"
  description = "Something isn't working"
}

resource "github_issue_label" "cla_no" {
  repository  = "${github_repository.repo.name}"
  name        = "cla: no"
  color       = "b60205"
  description = "Cannot accept contribution until Google CLA is signed."
}

resource "github_issue_label" "cla_yes" {
  repository  = "${github_repository.repo.name}"
  name        = "cla: yes"
  color       = "0e8a16"
  description = "Google CLA has been signed!"
}

resource "github_issue_label" "code_health" {
  repository  = "${github_repository.repo.name}"
  name        = "code health"
  color       = "bfd4f2"
  description = "Code health task, either refactoring or testing"
}

resource "github_issue_label" "documentation" {
  repository  = "${github_repository.repo.name}"
  name        = "documentation"
  color       = "edd782"
  description = "Documentation change"
}

resource "github_issue_label" "duplicate" {
  repository  = "${github_repository.repo.name}"
  name        = "duplicate"
  color       = "cfd3d7"
  description = "This issue or pull request already exists"
}

resource "github_issue_label" "enhancement" {
  repository  = "${github_repository.repo.name}"
  name        = "enhancement"
  color       = "a2eeef"
  description = "New feature or request"
}

# The "good first issue" label is treated specially by GitHub:
# https://help.github.com/en/articles/helping-new-contributors-find-your-project-with-labels
resource "github_issue_label" "good_first_issue" {
  repository  = "${github_repository.repo.name}"
  name        = "good first issue"
  color       = "7057ff"
  description = "Good for newcomers"
}

resource "github_issue_label" "process" {
  repository  = "${github_repository.repo.name}"
  name        = "process"
  color       = "a2aaef"
  description = "Improvement to the engineering process"
}

resource "github_issue_label" "in_progress" {
  repository  = "${github_repository.repo.name}"
  name        = "in progress"
  color       = "99ffad"
  description = "This is being actively worked on"
}

resource "github_issue_label" "needs_info" {
  repository  = "${github_repository.repo.name}"
  name        = "needs info"
  color       = "d876e3"
  description = "Further discussion or clarification is necessary"
}

resource "github_issue_label" "ready_to_submit" {
  repository  = "${github_repository.repo.name}"
  name        = "ready to submit"
  color       = "0e8a16"
  description = "Pull request has been approved and should be merged once tests pass"
}

resource "github_issue_label" "P0" {
  repository  = "${github_repository.repo.name}"
  name        = "P0"
  color       = "990000"
}

resource "github_issue_label" "P1" {
  repository  = "${github_repository.repo.name}"
  name        = "P1"
  color       = "ff6666"
}

resource "github_issue_label" "P2" {
  repository  = "${github_repository.repo.name}"
  name        = "P2"
  color       = "cc9900"
}
