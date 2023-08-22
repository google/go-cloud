#!/bin/bash

# To update the website:
#
# Install Hugo locally, by downloading a version (at least 0.92) and unpacking:
#
# In $HUGODIR:
#
#   wget https://github.com/gohugoio/hugo/releases/download/v0.91.2/hugo_0.91.2_Linux-64bit.tar.gz
#   tar xvf hugo_0.91.2_Linux-64bit.tar.gz
#
# This creates a binary $HUGODIR/hugo
#
# In a go-cloud clone, run:
#
#   $HUGODIR/hugo -s internal/website
#
# This updates the internal/website/public directory with the new contents of
# the website. Now we'll need a separate clone of go-cloud, with the gh-pages
# branch checked out:
#
#   git clone git@github.com:google/go-cloud.git GH-PAGES-CLONE
#   cd GH-PAGES-CLONE
#   git co gh-pages
#
# This should have the contents of the website (configured in
# https://github.com/google/go-cloud/settings/pages).
#
# Once that's ready, copy the contents of internal/website/public into the root
# directory of the clone that's on the gh-pages branch, e.g. with rsync:
#
#   rsync -avc internal/website/public/ GH-PAGES-CLONE
#
# Commit into the gh-pages branch and push it to origin (git push origin
# gh-pages). This deploys the new site contents.

# (Old)
# Here's what we had in Travis:
# install: "curl -fsSL https://github.com/gohugoio/hugo/releases/download/v0.54.0/hugo_0.54.0_Linux-64bit.tar.gz | tar zxf - -C \"$HOME\" hugo"
# script: "HUGO_GOOGLEANALYTICS=UA-135118641-1 \"$HOME/hugo\" -s internal/website"
# deploy:
#   provider: pages
#   edge: true
#   fqdn: gocloud.dev
#   skip-cleanup: true
#   local-dir: internal/website/public
