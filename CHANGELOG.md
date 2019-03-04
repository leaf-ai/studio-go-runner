# 0.7.1

IMPROVEMENTS:

* Added github templates for PRs, Issues etc. ([#133](https://github.com/SentientTechnologies/studio-go-runner/issue/133)).

* Capture artifact downloading failures and insert them into the experiments output file. ([#133](https://github.com/SentientTechnologies/studio-go-runner/issue/133)).

# 0.8.0

IMPROVEMENTS:

* Added support for testing and non-release builds using kubernetes hosted pods, please see docs/k8s.md.  Releasing from k8s hosting a future feature.

* RabbitMQ now supported within k8s testing as a seperate service within the namespace the test uses.  Please see docs/k8s.md for more information.

* Multiple test cases added, many more to go but a legitimate effort is now underway given we have k8s support and are not constrained by travis.

* Config map support within kubernentes to inform pods of desired state changes Running, Abort, Drain and suspend, Drain and terminate.  Enables rolling upgrade and maintenance use cases for k8s clusters.

# 0.8.1

IMPROVEMENTS:

* Faulty GPUs with bad ECC memory now caught and will only accept CPU jobs, in addition to errors being output

# 0.9.0

IMPROVEMENTS:

* GPUs can now be aggregated for experiments needing more than 1 card, or a large card.  Uses CUDA_VISIBLE_DEVICES.  Validated using pytorch.
* Live testing now added to CI/CD process involving real Multi and Single GPU jobs.

# 0.9.1

IMPROVEMENTS:

* 3rd party vendor directory license reporting added

# 0.9.2

BUG FIXES:

* Multi GPU setups used only the headroom of a single GPU when scanning for new work causing multi GPU experiments to be rejected after their first experiment was completed

# 0.9.3

IMPROVEMENTS:

* Remove slack support for logging as Kubernetes is now the base line operations platform

# 0.9.4

IMPROVEMENTS:

* Capture metadata lines from experiments and populate the \_metadata artifact with host, runner, and experiment outputs as keys within the artifact

# 0.9.5

IMPROVEMENTS:

* Migrate to the leaf-ai repository owner
* Add support for experiment JSON metadata artifacts with merge and patch RFC format fragments
* microk8s support for workstation and laptop full stack deployments

# 0.9.6

IMPROVEMENTS:

* Relocate the logging interface to the reusable library pkg location for leaf and other software components

# 0.9.7

IMPROVEMENTS:

* Migrate container tags to leaf-ai on public docker image repositories on Azure and AWS

FIXES:

* Fix an issue where empty lines would cause a JSON format check to get an out of bounds panic

# 0.9.8

IMPROVEMENTS:

* Add unauthenticated access for S3 to allow minio public folders with credentials for other S3 implementations to co-exist

FIXES:

* Fix for handling slow job termination

# 0.9.9

FIXES:

* Anonymous access to S3 and tests validating feature

# 0.9.10

IMPROVMENTS

* Image repository naming modified to work with dockerhub, images can now be pushed to the docker hub leafai account
* Git actions ready, changes to allow larger base containers to be prebuilt reducing build requirements in the Git infrastructure
* quay.io based builds from github commit/push on any branch
* keel.sh based CI with automatted builds and tests using git commit notifications

# 0.9.11

IMPROVEMENTS:

* quay.io image name for keel based CI now uses the branch name for the image tag

FIXES:

* repair dependabot mayhem that broke the builds and a tag removed from a 3rd party repository
