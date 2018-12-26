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

# This Dockerfile expects to have the context of the whole repository.

# Step 1: Build Go binary.
FROM golang:1.11.4 as build
ENV GO111MODULE on
COPY . /go/src/go-cloud
WORKDIR /go/src/go-cloud/internal/contributebot
RUN go build

# Step 2: Create image with built Go binary.
FROM debian:stretch-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    git-core \
  && rm -rf /var/lib/apt/lists/*
COPY --from=build /go/src/go-cloud/internal/contributebot/contributebot /
# Expose 8080 for health checks
EXPOSE 8080
ENTRYPOINT ["/contributebot"]
