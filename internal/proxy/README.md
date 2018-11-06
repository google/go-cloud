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
2.  Add the dependency to the GCS bucket:

```bash
go clean -modcache

# Run this command in the master branch.
# Run it again in your branch that's adding a new dependency.
./internal/proxy/makeproxy.sh proxy

# Synchronize your cache to the proxy.

# -n: preview only
# -r: recurse directories
# -c: compare checksums not write times
gsutil rsync -n -r -c ${GOPATH}/pkg/mod/cache/download gs://go-cloud-modules
# If the set of packages being looks good, repeat without the -n.

# Optionally, you can run with -d, which deletes remote files that aren't
# present locally. We should do this periodically to trim our proxy bucket.
```
