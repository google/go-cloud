# Releases

To do a release:

-   Pick the new release name; it's probably `v0.x.0` where `x` is whatever the
    [last release](https://github.com/google/go-cloud/releases/latest) was plus
    one, but follow [semantic versioning](https://semver.org/).

-   Consider updating dependencies via `internal/testing/update_deps.sh` if it
    hasn't been done recently. Do this as a separte step before the release.

-   Create a new branch for the release (`git checkout -B prerelease`).

-   Update the `User-Agent` version in internal/useragent/useragent.go.

-   Run the release helper tool to remove `replace` lines from the `go.mod`
    files of submodules:

    ```bash
    $ go run internal/releasehelper/releasehelper.go dropreplace
    ```

-   Run the release helper tool to set the version in `require` directives of
    submodules to the new (yet unreleased) version:

    ```bash
    $ go run internal/releasehelper/releasehelper.go setversion v0.x.0
    ```

-   Commit and create a PR. Tests will fail for this PR because submodules depend on a
    version of the main module that wasn't tagged yet, so you may have to
    force-merge the PR. Note that this does not affect users, since a new
    version hasn't been tagged yet.

-   `git sync` your local client and move to the master branch.

-   Tag new versions by running `./internal/testing/git_tag_modules.sh v0.X.0`.

-   Push tags to upstream with `git push upstream --tags`

-   Go to [Releases](https://github.com/google/go-cloud/releases). Click `Draft
    a new release`, enter your release name, select your tag from the dropdown,
    and enter release notes by clicking "Generate Release Notes".

    -   Add a section for breaking changes, if any. They should be marked with
        `BREAKING_CHANGE` in the PR title; however, that's not enforced so do
        your best to look for them.
    -   Update the list of changes to remove anything that's not interesting
        (e.g., updating dependencies, prerelease, minor cleanups, etc.).

-   Send an email to
    [go-cloud@googlegroups.com](https://groups.google.com/forum/#!forum/go-cloud)
    announcing the release, and including the release notes.

-   Create a new branch for the postrelease (`git checkout -B postrelease`).

-   Add back `replace` lines:

    ```bash
    $ go run internal/releasehelper/releasehelper.go addreplace
    ```

    Run tests and send out a PR as usual.
