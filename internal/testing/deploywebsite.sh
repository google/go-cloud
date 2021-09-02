#!/bin/bash

# TODO implement.
# Here's what we had in Travis:
# install: "curl -fsSL https://github.com/gohugoio/hugo/releases/download/v0.54.0/hugo_0.54.0_Linux-64bit.tar.gz | tar zxf - -C \"$HOME\" hugo"
# script: "HUGO_GOOGLEANALYTICS=UA-135118641-1 \"$HOME/hugo\" -s internal/website"
# deploy:
#   provider: pages
#   edge: true
#   fqdn: gocloud.dev
#   skip-cleanup: true
#   local-dir: internal/website/public

# To do this manually:
#
# Invoke the `hugo` command manually (after installing `hugo`).
# Separately check out the gh-pages branch of this repository.
# Overwrite the contents of the branch with the public/ directory, and push
# to the branch.
