# Releases

We aim to do a release after each Sprint (roughly, every 3 weeks).

To do a release:

-   Identify the [commit](https://github.com/google/go-cloud/commits/master) you
    want to label as the release; it should be a commit from around the end of
    the last Sprint.
-   Pick the new release tag; it's probably `v0.x.0` where `x` is whatever the
    last release was plus one, but follow
    [semantic versioning](https://semver.org/).
-   Compile the release notes for this release by reviewing the commits since
    the last release and listing highlights, in the form: `**<component>**:
    <description of change, past tense>`. For example, `**blob**: Added feature
    foo.`.
-   Tag the commit:

```bash
$ TAG=v0.n.0
$ COMMIT=aaaaaa
$ git tag -a ${TAG} ${COMMIT}
# Enter your release notes in your editor.
$ git push upstream ${TAG}
```

-   Go to [Releases](https://github.com/google/go-cloud/releases). Click `Draft
    a new release` and select your new tag. Enter the release notes again.

-   Send an email to the external mailing list announcing the release, and
    including the release notes.
