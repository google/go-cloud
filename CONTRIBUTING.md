# How to Contribute

We'd love to accept your patches and contributions to this project. Here's how you can help.

## Contributor License Agreement

Contributions to this project can only be made by those who have signed Google's Contributor License
Agreement. You (or your employer) retain the copyright to your contribution,
this simply gives us permission to use and redistribute your contributions as
part of the project. Head over to <https://cla.developers.google.com/> to see
your current agreements on file or to sign a new one.

As a personal contributor, you only need to sign the Google CLA once across all Google projects. If you've already signed the CLA, there is no need to do it again. If you are submitting code on behalf of your employer, there's [a separate corporate CLA that your employer manages for you](https://opensource.google.com/docs/cla/#external-contributors).

## Pull requests
The most direct way to get a feature into the Go X Cloud repository is by sending us a [Pull Request](https://help.github.com/articles/about-pull-requests/). All Go X Cloud work is performed on GitHub.

### READ FIRST: A warning about tests
Tests that touch cloud providers are written using a replay method, where the test is run and actually performs the operations, and the requests/results of the operations are stored in a replay file. This replay file is then read back in unit tests, so the tests get to operate with real data. Unfortunately, while we can generate these replay files on our end against our test cloud infrastructure, it is not yet possible for you to do the same. Write your tests how you expect the change should work, and send out the PR even though the tests will fail (because the replay file doesn't have the righe right requests/responses in it). We can generate the replay files for you on your pull request, but this means that you may not be able to properly test your change before sending it to us.

We will improve this process in the future, and we are researching how we can do this. If you have any ideas, please file an issue!

### Making a pull request
1. Fork the [repository](https://github.com/google/go-x-cloud).
1. Clone your repository to your local machine.
1. Optional, but recommended, is to make a new feature branch to put your change in using `git checkout -b my-cool-feature`
1. Code! Feel free to make as many commits as you want; we will squash them all into a single commit before merging your change.
1. Build your changes using `vgo build .`, _do not use `go`_. Go X Cloud only supports `vgo` in order to ensure reproducible builds.
1. Test your changes using `vgo test -short .`. Always use `-short`, as that will use the replay files instead of attempting to reach out to a cloud provider. If your change _doesn't_ require the use of a replay file, please add tests that show the change does what it says it does, even if there wasn't a test in the first place. 
1. Push your branch to GitHub using `git push origin my-cool-feature`.
1. Head to your GitHub repository, which will show a useful banner asking you if you want to make a pull request from your recently pushed branch. Click it!
1. Check the diffs, write a useful description (including something like `Fixes #123` if it's fixing a bug) and send the PR out.
1. [Travis CI](http://travis-ci.com) will run tests against the PR. This should happen within 10 minutes or so. If the tests pass, please assign a reviewer from the Go X Cloud team (@zombiezen, @shantuo, @cflewis) who will start the review process. If a test fails, go back to the coding stage and try to fix the test and push the branch again. You won't need to make a new pull request, the changes will be rolled directly into the PR you already opened. Wait for Travis again. 
1. Once your PR is approved (hooray!) the reviewer will squash your commits into a single commit, and then merge the commit onto the Go X Cloud master branch. Thank you!

### Code review
All submissions, including submissions by project members, require review. It is almost never the case that a pull request is accepted without some changes requested, so please do not be offended!

When you have finished making requested changes to your pull request, please make
a comment containing "PTAL" (Please Take Another Look) on your pull request.
GitHub notifications can be noisy, and it is unfortunately easy for things to be lost in the shuffle.

### Finding something to work on
If you want to write some code, but don't know where to start or what you might want to do, take a look at our **Unplanned** milestone. This is where you can find issues we'd like to address but can't currently find time for. See if any of the latest ones look interesting! If you need help before you can start work, you can comment on the issue and we will try to help as best we can.

## Filing issues
Filing issues is another important way you can contribute to the Go X Cloud Project. We want your feedback on things like bugs, desired API changes, or just anything that isn't working for you.

### Bugs
If your issue is a bug, the easiest way to file an issue with all the right information is to run `go bug`. `go bug` will print out a handy template of questions and system information that will help us get to the root of the issue quicker.

### Changes
Unlike the core Go project, we do not have a formal proposal process for changes. If you have a change you would like to see in Go X Cloud, please go ahead and file an issue with the necessary details.

### Triaging
The Go X Cloud team triage issues at least every two weeks, but usually within two business days. Bugs or feature requests are either placed into a **Sprint** milestone which means the issue is intended to be worked on. Issues that we would like to address but do not have time for are placed into the **Unplanned** milestone.
