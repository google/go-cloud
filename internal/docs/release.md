# Releases

To do a release:

-   Make sure all tests pass localy with `runchecks.sh` and Travis is green.
-   Run the prerelease checks until everything is passing.
    -   Create a new branch (`git checkout -B prerelease`). Running the script
        will modify hundreds of golden files, so you want to do it in a branch.
        You can delete the branch and abandon all the changes when you are done.
    -   Run `./internal/testing/prerelease.sh init` until it finishes with
        `SUCCESS`. Note that there are some known failures that are logged but
        don't prevent overall `SUCCESS`.
    -   Run `./internal/testing/prerelease.sh run` Again, there are some known
        failures. You can re-run any unexpected failures independently to
        investigate; see the `prerelease.sh` script to see the exact command
        line to use.
    -   Run `./internal/testing/prerelease.sh cleanup` when you are done.
    -   Delete the branch (`-D` to force deleting with uncommitted changes).

-   Run the release helper tool to remove `replace` lines from the `go.mod`
    files of submodules:

    ```bash
    $ go run internal/releasehelper/releasehelper.go dropreplace
    ```

    Check that everything looks in order (with `git diff`).

-   Pick the new release name; it's probably `v0.x.0` where `x` is whatever the
    [last release](https://github.com/google/go-cloud/releases/latest) was plus
    one, but follow [semantic versioning](https://semver.org/).

-   Run the release helper tool to set the version in `require` directives of
    submodules to the new (yet unreleased) version:

    ```bash
    $ go run internal/releasehelper/releasehelper.go setversion v0.x.0
    ```

    Check that everything looks in order (with `git diff`).

-   Commit and create a PR. Travis will fail for this PR because submodules
    depend on a version of the main module that wasn't tagged yet. Enable force
    merging in GitHub settings and force-merge the PR. Note that this does not
    affect users, since a new version hasn't been tagged yet.

-   Tag new versions for all released in Git (TODO(eliben): script this?):

    ```bash
    $ git tag v0.x.0
    $ git tag secrets/vault/v0.x.0
    $ git tag runtimevar/etcdvar/v0.15.0
    ...
    ```

-   Push tags to upstream with `git push upstream --tags`

-   Compile release notes by reviewing the commits since the last release (use
    the [Compare](https://github.com/google/go-cloud/compare/v0.1.1...v0.2.0)
    page for this).
    -   Put breaking changes in a separate section. They should be marked with a
        `BREAKING_CHANGE` in the PR title; however, that's not enforced so do
        your best to look for them.
    -   List highlights in the form: `**<component>**: <description of change,
        past tense>`. For example, `**blob**: Added feature foo.`.
-   Go to [Releases](https://github.com/google/go-cloud/releases). Click `Draft
    a new release`, enter your release name, select your tag from the dropdown,
    and enter your release notes.

-   Send an email to
    [go-cloud@googlegroups.com](https://groups.google.com/forum/#!forum/go-cloud)
    announcing the release, and including the release notes.
