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
2.  Gather the new set of dependencies and sync them to the GCS bucket by running
    `internal/proxy/add.sh` from the root of the Go Cloud repository.

