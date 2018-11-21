# How to Contribute

We would love to accept your patches and contributions to this project. Here is
how you can help.

## Filing issues

Filing issues is an important way you can contribute to the Go Cloud Project. We
want your feedback on things like bugs, desired API changes, or just anything
that isn't working for you.

### Bugs

If your issue is a bug, open one
[here](https://github.com/google/go-cloud/issues/new). The easiest way to file
an issue with all the right information is to run `go bug`. `go bug` will print
out a handy template of questions and system information that will help us get
to the root of the issue quicker. Please start the title of your bug with the
name of the affected package, followed by a colon, followed by a short summary
of the issue, like "blob/gcsblob: not blobby enough".

### Changes

Unlike the core Go project, we do not have a formal proposal process for
changes. If you have a change you would like to see in Go Cloud, please file an
issue with the necessary details.

### Triaging

The Go Cloud team triages issues at least every two weeks, but usually within
two business days. Bugs or feature requests are either placed into a **Sprint**
milestone which means the issue is intended to be worked on. Issues that we
would like to address but do not have time for are placed into the [Unplanned][]
milestone.

[Unplanned]: https://github.com/google/go-cloud/milestone/2

## Contributing Code

We love accepting contributions! If your change is minor, please feel free
submit a [pull request](https://help.github.com/articles/about-pull-requests/).
If your change is larger, or adds a feature, please file an issue beforehand so
that we can discuss the change. You're welcome to file an implementation pull
request immediately as well, although we generally lean towards discussing the
change and then reviewing the implementation separately.

Be sure to take a look at the [internal docs][], which include more information
about conventions and design patterns found in the project.

[internal docs]: internal/docs/README.md

### Finding something to work on

If you want to write some code, but don't know where to start or what you might
want to do, take a look at our [Unplanned][] milestone. This is where you can
find issues we would like to address but can't currently find time for. See if
any of the latest ones look interesting! If you need help before you can start
work, you can comment on the issue and we will try to help as best we can.

### Contributor License Agreement

Contributions to this project can only be made by those who have signed Google's
Contributor License Agreement. You (or your employer) retain the copyright to
your contribution, this simply gives us permission to use and redistribute your
contributions as part of the project. Head over to
<https://cla.developers.google.com/> to see your current agreements on file or
to sign a new one.

As a personal contributor, you only need to sign the Google CLA once across all
Google projects. If you've already signed the CLA, there is no need to do it
again. If you are submitting code on behalf of your employer, there's
[a separate corporate CLA that your employer manages for you](https://opensource.google.com/docs/cla/#external-contributors).

### A Warning About Tests

Tests that interact with cloud providers are written using a replay method,
where the test is run and actually performs the operations, and the
requests/results of the operations are stored in a replay file. This replay file
is then read back in unit tests on Travis, so the tests get to operate with real
data. Unfortunately, while the Go Cloud team can generate these replay files
against our test cloud infrastructure, it is not yet possible for external
contributors to do the same. We want to improve this process in the future and
are researching how we can do this. If you have any ideas, please file an issue!

#### Writing and running tests against a cloud environment

If you can create cloud resources, setup your authentication using either `aws`
or `gcloud` and set the default project as the test project. Most tests will use
this project, unfortunately
[some tests need a flag](https://github.com/google/go-cloud/issues/128) and will
fail telling you what it needs to be passed. You can then run `go test` and a
new replay file is generated. This uses API quota and will create and delete
cloud resources. Replay files scrub sensitive information.
[Send your PR](#making-a-pull-request) without the replay files, and we can
generate new ones to be used by others.

#### Writing tests locally

While we figure out how to properly generate replay files for contributors,
write your tests how you expect the change should work, and send out the PR even
though the tests will fail (because the replay file doesn't have the right
requests/responses in it). We can generate the replay files for you on your pull
request, but this means that you may not be able to properly test your change
before sending it to us, and please accept our apologies for the bad experience
right now.

## Making a pull request

*   Follow the normal
    [pull request flow](https://help.github.com/articles/creating-a-pull-request/)
*   Build your changes using Go 1.11 with Go modules enabled. Go Cloud's
    continuous integration uses Go modules in order to ensure
    [reproducible builds](https://research.swtch.com/vgo-repro).
*   Test your changes using `go test -short .`. You can omit the `-short` if you
    actually want to test your change against a cloud platform and you have a
    permissions level that enable you to do so. Please add tests that show the
    change does what it says it does, even if there wasn't a test in the first
    place. Don't add the replay files to your commits.
*   Feel free to make as many commits as you want; we will squash them all into
    a single commit before merging your change.
*   Check the diffs, write a useful description (including something like
    `Fixes #123` if it's fixing a bug) and send the PR out. Please start the
    title of your pull request with the name of the affected package, followed
    by a colon, followed by a short summary of the change, like "blob/gcsblob:
    add more tests".
*   [Travis CI](http://travis-ci.com) will run tests against the PR. This should
    happen within 10 minutes or so. If a test fails, go back to the coding stage
    and try to fix the test and push the same branch again. You won't need to
    make a new pull request, the changes will be rolled directly into the PR you
    already opened. Wait for Travis again. There is no need to assign a reviewer
    to the PR, the project team will assign someone for review during the
    standard [triage](#triaging) process.

## Code review

All submissions, including submissions by project members, require review. It is
almost never the case that a pull request is accepted without some changes
requested, so please do not be offended!

When you have finished making requested changes to your pull request, please
make a comment containing "PTAL" (Please Take Another Look) on your pull
request. GitHub notifications can be noisy, and it is unfortunately easy for
things to be lost in the shuffle.

Once your PR is approved (hooray!) the reviewer will squash your commits into a
single commit, and then merge the commit onto the Go Cloud master branch. Thank
you!

### Squashing

(This section is for project members.)

When you are squashing-and-merging a pull request into master, please edit the
commit message to fit in with the [project conventions][commit messages]. In
particular:

-   Ensure that the first line summary starts with the primary package affected,
    followed by a colon, followed by a short summary of the change, like
    "blob/gcsblob: made more blobby (#123)".
-   If the change is addressing an issue, make the commit message end with a
    block of "Updates #123" and/or "Fixes #456", one issue number per line.
-   Don't include the automatically added code-review commit lines like "Fixed
    some stuff" or "Addressed feedback".

[#317][] and [#318][] are tracking improvements to Contribute Bot to
automatically warn for some of these problems.

[#317]: https://github.com/google/go-cloud/issues/317
[#318]: https://github.com/google/go-cloud/issues/318
[commit messages]: https://golang.org/doc/contribute.html#commit_messages
