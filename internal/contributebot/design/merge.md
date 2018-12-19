# Contribute Bot Auto-Merge Design

Author: [@zombiezen][]<br>
Date: 2018-12-17<br>
Part of [#687][]

[@zombiezen]: https://github.com/zombiezen
[#687]: https://github.com/google/go-cloud/issues/687

## Objective

As a maintainer of Go Cloud, it is desirable to have pull requests merged in as
quickly as they can be merged after receiving approvals, assuming there are no
test breakages or merge conflicts. Currently, merging requires a manual process:

```
while branch is not up-to-date with master {
  press update branch button
  wait for Travis run to finish
}
click squash-and-merge
```

This loop is especially expensive the longer the waiting is, since it increases
the likelihood that the branch is not up-to-date and thus causing the cycle to
increase iterations (more waiting!).

The objective of this design is to reduce the amount of maintainer [toil][] in
this process.

[toil]: https://landing.google.com/sre/sre-book/chapters/eliminating-toil/

## Overview

To reduce maintainer toil, this document proposes a new set of interactions for
Contribute Bot. If a user writes a comment on a pull request that starts with
the text "/merge", Contribute Bot will:

1.  Verify that the commenter has `write` or `admin` access to the repository.
    If they do not, do not proceed and leave an error comment on the PR.
2.  Verify that the pull request has the approvals required by the [GitHub
    branch protections][required reviews]. If it does not, do not proceed and
    leave an error comment on the PR.
3.  Verify that there are no other commits on the pull request branch since the
    last approved commit (or a manually entered commit hash entered by the
    commenter) other than those created by Contribute Bot. If such commits
    exist, then do not proceed and leave a comment on the PR mentioning that
    "/merge COMMITHASH" will bypass this. While this is stricter than what the
    branch protections might allow, it avoids accidentally merging in unreviewed
    changes.
3.  Leave a comment on the PR to the effect that Contribute Bot has received the
    request and that it will report back if it is unable to merge the PR.
4.  If the pull request is not up to date with the changes on the base branch,
    then create a merge commit on the pull request branch. If the merge commit
    cannot be created due to merge conflicts, then stop trying to merge the PR
    and make an error comment on the PR.
5.  Wait for any necessary check runs to report completion. If any report
    failure, then stop trying to merge the PR and make an error comment on the
    PR.
6.  Merge the pull request using the project's allowed merge method (`squash`
    for Go Cloud and Wire), using the title and body of the pull request (the
    first comment) as the commit message in the case of `merge` or `squash`.

Multiple pull requests may be requested to be merged at the same time. In this
case, Contribute Bot will hold off on proceeding through Steps 4-6 until the
merge requests made earlier have either been merged or stopped due to errors.

[required reviews]: https://help.github.com/articles/about-required-reviews-for-pull-requests/

## Detailed Design

TODO

## Security Considerations

Contribute Bot will need the following new permissions:

-   [Read & write repository contents][] (writing the merge commit)
-   [Read repository administration][] (identifying which check runs are necessary to pass branch protections)

[Read repository administration]: https://developer.github.com/v3/apps/permissions/#permission-on-administration
[Read & write repository contents]: https://developer.github.com/v3/apps/permissions/#permission-on-contents

## Testing Plan

This will be tested just like Contribute Bot is tested currently: by running
manual tests against a sandbox repository.

## Alternatives Considered

There are a handful of different off-the-shelf solutions that act as GitHub
merge bots, but each would need to be customized to suit Go Cloud's needs. It
would also increase the number of systems that the Go Cloud contributors would
need to maintain and monitor.

This could also be implemented as a tool that each maintainer individually
runs. However, this would require an always-on connection and additional
per-maintainer burden, versus having a centralized implementation that's already
connected and properly authorized to the project. Having a centralized
service merge the pull requests also reduces load on Travis, since it can hold
off on doing unnecessary merges while other pull requests are queued for merging.
