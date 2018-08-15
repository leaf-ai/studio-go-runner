# Contributing to `studio-go-runner`

`studio-go-runner` is an open source project.

It originated within Sentient Technologies and is now open for others to use. We hope that it offers value to those who engage with the project.  We appreciate your help!

## Filing issues
<!---
Please check the existing issues and [FAQ](docs/FAQ.md) to see if your feedback has already been reported.
-->

Please check the existing issues to see if your feedback has already been reported, or your question has been answered.

When [filing an issue](https://github.com/SentientTechnologies/studio-go-runner/issues/new), make sure to answer these five questions:

1. What version of `studio-go-runner` are you using??
3. What command line parameters and environment variables did you use?
4. What did you expect to see?
5. What did you see instead?

## Contributing code

Let us know if you are interested in working on an issue by leaving a comment
on the issue in GitHub. This helps avoid multiple people unknowingly 
working on the same issue.

The
[help wanted](https://github.com/SentientTechnologies/studio-go-runner/issues?q=is%3Aissue+is%3Aopen+label%3A%22help%20wanted%22)
label highlights issues that are well-suited for folks to jump in on. The
[good first issue](https://github.com/SentientTechnologies/studio-go-runner/issues?q=is%3Aissue+is%3Aopen+label%3A%22good%20first%20issue%22)
label further identifies issues that are particularly well-sized for newcomers.

Unless otherwise noted, the `studio-go-runner` source files are distributed under
the permissive Apache License found in the LICENSE file.

All submissions, require review. We use GitHub pull requests for this purpose. 
Consult [GitHub Help] for more information on using pull requests.

We check `studio-go-runner`'s own `vendor` directory into git. For any PR to `studio-go-runner` where you're
updating `Gopkg.toml`, make sure to run `dep ensure` and commit all changes to `vendor`.

[GitHub Help]: https://help.github.com/articles/about-pull-requests/

## Contributing to the Documentation

All the docs reside in the [`docs/`](docs/) directory. For any relatively small
change - like fixing a typo or rewording something - the easiest way to
contribute is directly on Github, using their web code editor.

<!---
For relatively big change - changes in the design, links or adding a new page -
the docs site can be run locally. We use [docusaurus](http://docusaurus.io/) to
generate the docs site. [`website/`](website/) directory contains all the
docusaurus configurations. To run the site locally, `cd` into `website/`
directory and run `npm i --only=dev` to install all the dev dependencies. Then
run `npm start` to start serving the site. By default, the site would be served
at http://localhost:3000.

## Contributor License Agreement

Contributions to this project must be accompanied by a Contributor License
Agreement. You (or your employer) retain the copyright to your contribution,
this simply gives us permission to use and redistribute your contributions as
part of the project. Head over to <https://cla.developers.google.com/> to see
your current agreements on file or to sign a new one.

You generally only need to submit a CLA once, so if you've already submitted one
(even if it was for a different project), you probably don't need to do it
again.
-->

## Maintainer's Guide

`studio-go-runner` has subsystem maintainers; this guide is intended for them in performing their work as a maintainer.

### General guidelines

* _Be kind, respectful, and inclusive_. <!--- Really live that [CoC](https://github.com/golang/dep/blob/master/CODE_OF_CONDUCT.md). We've developed a reputation as one of the most welcoming and supportive project environments in the Go community, and we want to keep that up!-->
* The lines of responsibility between maintainership areas can be fuzzy. Get to know your fellow maintainers - it's important to work _with_ them when an issue falls in this grey area.
* Being a maintainer doesn't mean you're always right. Admitting when you've made a mistake keeps the code flowing, the environment health, and the respect level up.
* It's fine if you need to step back from maintainership responsibilities - just, please, don't fade away! Let other maintainers know what's going on.


### Pull Requests

* Try to make, and encourage, smaller pull requests.
* [No is temporary. Yes is forever.](https://blog.jessfraz.com/post/the-art-of-closing/)
* Long-running feature branches should generally be avoided. Discuss it with maintainers first.
* Checklist for merging PRs:
  * Does the PR pass.
  * Are there tests to cover new or changed behavior? Prefer reliable tests > no tests > flaky tests.
  * Does the first post in the PR contain "Fixes #..." text for any issues it resolves?
  * Are any necessary follow-up issues _already_ posted, prior to merging?
  * Does this change entail the updating of any docs?
     * For docs kept in the repo, e.g. FAQ.md, docs changes _must_ be submitted as part of the same PR.
