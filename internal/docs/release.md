# Releases

We aim to do a release after each Sprint (roughly, every 3 weeks).

To do a release:

-   Identify the [commit](https://github.com/google/go-cloud/commits/master) you
    want to label as the release; it should be a commit from around the end of
    the last Sprint.
-   Pick the new release name; it's probably `v0.x.0` where `x` is whatever the
    [last release](https://github.com/google/go-cloud/releases/latest) was plus
    one, but follow [semantic versioning](https://semver.org/).
-   Compile release notes by reviewing the commits since the last release (use
    the [Compare](https://github.com/google/go-cloud/compare/v0.1.1...v0.2.0)
    page for this), and listing highlights in the form: `**<component>**:
    <description of change, past tense>`. For example, `**blob**: Added feature
    foo.`.
-   Go to [Releases](https://github.com/google/go-cloud/releases). Click `Draft
    a new release`, enter your release name, select your tag from the dropdown,
    and enter your release notes. Note that Github only has the last few commits
    in the dropdown; if your commit is older than what it shows, you'll need to
    create a tag from the command line and then select it in the UI.

```bash
$ TAG=v0.n.0
$ COMMIT=aaaaaa
$ git tag -a ${TAG} -m ${TAG} ${COMMIT}
$ git push upstream ${TAG}
```

-   Send an email to
    [go-cloud@googlegroups.com](https://groups.google.com/forum/#!forum/go-cloud)
    announcing the release, and including the release notes.
