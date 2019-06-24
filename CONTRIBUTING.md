# How to Contribute

We would love to accept your patches and contributions to this project. Here is
how you can help.

## Filing issues

Filing issues is an important way you can contribute to the Go Cloud Development
Kit. We want your feedback on things like bugs, desired API changes, or just
anything that isn't working for you.

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
changes. If you have a change you would like to see in the Go CDK, please file
an issue with the necessary details.

### Triaging

The Go CDK team triages issues at least every two weeks, but usually within two
business days. Issues that we would like to address, but do not have time for
are placed into the [Unplanned][] milestone.

[Unplanned]: https://github.com/google/go-cloud/milestone/2

## Contributing Code

We love accepting contributions! If your change is minor, please feel free
to submit a [pull request](https://help.github.com/articles/about-pull-requests/).
If your change is larger, or adds a feature, please file an issue beforehand so
that we can discuss the change. You're welcome to file an implementation pull
request immediately as well, although we generally lean towards discussing the
change and then reviewing the implementation separately.

Be sure to take a look at the [internal docs][], which include more information
about conventions and design patterns found in the project.

[internal docs]: internal/docs/README.md

### Finding something to work on

If you want to write some code, but don't know where to start or what you might
want to do, take a look at the [Good First Issue] label and our [Unplanned][]
milestone. The latter is where you can find issues we would like to address, but
can't currently find time for. See if any of the latest ones look interesting!
If you need help before you can start work, you can comment on the issue, and we
will try to help as best we can.

[Good First Issue]: https://github.com/google/go-cloud/labels/good%20first%20issue

### Contributor License Agreement

Contributions to this project can only be made by those who have signed Google's
Contributor License Agreement. You (or your employer) retain the copyright to
your contribution, so this simply gives us permission to use and redistribute your
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
data. Unfortunately, while the Go CDK team can generate these replay files
against our test cloud infrastructure, it is not yet possible for external
contributors to do the same. We want to improve this process in the future and
are researching how we can do this. If you have any ideas, please 
[file an issue](https://github.com/google/go-cloud/issues/new)!

#### Writing and running tests against a cloud environment

If you can create cloud resources, setup your authentication using either `aws`
or `gcloud` and set the default project as the test project. Most tests will
have constants defining the resource names they use (for example, for `blob`
tests, the bucket name); update the constant and then run `go test -record`. New
replay files will be generated. This uses API quota and will create and delete
cloud resources. Replay files scrub sensitive information.
[Send your PR](#making-a-pull-request) without the replay files, and we can
generate new ones to be used by others.

### Dependencies

The Go CDK has a policy to depend only on code licensed under one of the
[`notice`][notice licenses], [`permissive`][permissive licenses], or
[`unencumbered`][unencumbered licenses] categories in the
[Google Open Source Licenses][] documentation. This is enforced with a
Travis build check that verifies that every dependency is in the
[`alldeps` file][]. Do not add new direct or indirect dependencies to the Go CDK
unless you have verified that the dependency is released under an acceptable
license.

[`alldeps` file]: https://github.com/google/go-cloud/blob/master/internal/testing/alldeps
[notice licenses]: https://opensource.google.com/docs/thirdparty/licenses/#notice
[permissive licenses]: https://opensource.google.com/docs/thirdparty/licenses/#permissive
[unencumbered licenses]: https://opensource.google.com/docs/thirdparty/licenses/#unencumbered
[Google Open Source Licenses]: https://opensource.google.com/docs/thirdparty/licenses/

## Making a pull request

*   Follow the normal
    [pull request flow](https://help.github.com/articles/creating-a-pull-request/).
*   Build your changes using Go 1.11 with Go modules enabled. The Go CDK's
    continuous integration uses Go modules in order to ensure
    [reproducible builds](https://research.swtch.com/vgo-repro).
*   Test your changes using `go test ./...`. Please add tests that show the
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

Once your PR is approved (hooray!), the reviewer will squash your commits into a
single commit and then merge the commit onto the Go CDK master branch. Thank
you!

## Github code review workflow conventions

(For project members and frequent contributors.)

As a contributor:

-   Try hard to make each Pull Request as small and focused as possible. In
    particular, this means that if a reviewer asks you to do something that is
    beyond the scope of the Pull Request, the best practice is to file another
    issue and reference it from the Pull Request rather than just adding more
    commits to the existing PR.
-   Adding someone as a Reviewer means "please feel free to look and comment";
    the review is optional. Choose as many Reviewers as you'd like.
-   Adding someone as an Assignee means that the Pull Request should not be
    submitted until they approve. If you choose multiple Assignees, wait until
    all of them approve. It is fine to ask someone if they are OK with being
    removed as an Assignee.
    -   Note that if you don't select any assignees, ContributeBot will turn all
        of your Reviewers into Assignees.
-   Make as many commits as you want locally, but try not to push them to Github
    until you've addressed comments; this allows the email notification about
    the push to be a signal to reviewers that the PR is ready to be looked at
    again.
-   When there may be confusion about what should happen next for a PR, be
    explicit; add a "PTAL" comment if it is ready for review again, or a "Please
    hold off on reviewing for now" if you are still working on addressing
    comments.
-   "Resolve" comments that you are sure you've addressed; let your reviewers
    resolve ones that you're not sure about.
-   Do not use `git push --force`; this can cause comments from your reviewers
    that are associated with a specific commit to be lost. This implies that
    once you've sent a Pull Request, you should use `git merge` instead of `git
    rebase` to incorporate commits from the master branch.
-   Travis checks will fail if you haven't run `gofmt -w -s`.
-   Travis checks will fail if your PR has backwards-incompatible changes,
    unless one of your commits has the strings `BREAKING_CHANGE_OK` in the first
    line of the commit message.

As a reviewer:

-   Be timely in your review process, especially if you are an Assignee.
-   Try to use `Start a Review` instead of single comments, to reduce email
    spam.
-   "Resolve" your own comments if they have been addressed.
-   If you want your review to be blocking, and are not currently an Assignee,
    add yourself as an Assignee.

When squashing-and-merging:

-   Ensure that **all** of the Assignees have approved.
-   Do a final review of the one-line PR summary, ensuring that it meets the
    guidelines (e.g., "blob: add more blobbing") and accurately describes the
    change.
-   Mark breaking changes with `BREAKING_CHANGE` in the commit message (e.g.,
    "blob: BREAKING_CHANGE remove old blob").
    -   If the PR includes a breaking change, it will be declared via a commit
        with `BREAKING_CHANGE_OK` in it (see Contributor section above).
    -   You can omit the marker if the change is technically breaking, but not
        expected to affect users (e.g., it's a breaking change to an object that
        wasn't in the last tagged release, or it's a change to a portable API
        function that's only expected to be used by driver implementations).
-   Delete the automatically added commit lines; these are generally not
    interesting and make commit history harder to read.
