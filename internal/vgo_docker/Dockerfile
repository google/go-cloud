# Copyright 2018 Google LLC
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

# Dockerfile for a Container Builder vgo build step.

# Initial stage: download go using curl and vgo using git.
FROM debian:stretch

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    git-core \
    mercurial \
  && rm -rf /var/lib/apt/lists/*
RUN curl -fsSL https://dl.google.com/go/go1.10.3.linux-amd64.tar.gz \
  | tar -C /usr/local -xzf -
RUN git clone https://go.googlesource.com/vgo /gopath/src/golang.org/x/vgo \
  && git -C /gopath/src/golang.org/x/vgo checkout 0f3e556044ecbd8a0805d098e52a743a1d0e3566 \
  && GOPATH=/gopath /usr/local/go/bin/go build -o /usr/local/bin/vgo golang.org/x/vgo \
  && rm -rf /gopath

ENV PATH /usr/local/go/bin:/usr/local/bin:/usr/bin

# vgo uses $GOPATH/v as cache.
VOLUME /gopath
ENV GOPATH /gopath
LABEL "github.com/google/go-cloud/go-version"="1.10.3"
LABEL "github.com/google/go-cloud/vgo-commit"="0f3e556044ecbd8a0805d098e52a743a1d0e3566"

ENTRYPOINT ["vgo"]
