# Go Cloud module proxy

The Travis build for Go Cloud uses a [Go module proxy][] for dependencies, for
efficiency and reliability, and to enforce that all of our dependencies meet our
license requirements.

The proxy is set
[here](https://github.com/google/go-cloud/blob/master/.travis.yml#L22).

[Go module proxy]: https://research.swtch.com/vgo-module

## Updating the GCS bucket

When you add a new dependency, the Travis build will fail. To add the new
dependency to the proxy:

1.  Ensure that the new dependency is covered under one of the `notice`,
    `permissive`, or `unencumbered` licences under the categories defined by
    [Google Open Source](https://opensource.google.com/docs/thirdparty/licenses/).
2.  Gather the new set of dependencies and sync them to the GCS bucket.

```bash
# This approach was suggested by:
# https://github.com/go-modules-by-example/index/tree/master/012_modvendor

# Create a temporary directory where we'll create a module download cache.
tgp=$(mktemp -d)

# Run this command in the master branch.
# It runs "go mod download" in every module that we have in our repo,
# filling the cache with all of the module dependencies we need.
GOPROXY=file://$GOPATH/pkg/mod/cache/download GOPATH=$tgp ./internal/proxy/makeproxy.sh

# Run the above command again in your branch that's adding a new dependency,
# to ensure the cache has any new dependencies.

# Move the temporary cache to modvendor/.
rm -rf modvendor
cp -rp $GOPATH/pkg/mod/cache/download/ modvendor

# Clean up the temporary cache.
GOPATH=$tgp go clean -modcache
rm -rf $tgp

# Synchronize modvendor to the proxy.

# -n: preview only
# -r: recurse directories
# -c: compare checksums not write times
gsutil rsync -n -r -c modvendor gs://go-cloud-modules
# If the set of packages being looks good, repeat without the -n.

# Optionally, you can run with -d, which deletes remote files that aren't
# present locally. We should do this periodically to trim our proxy bucket.
```
