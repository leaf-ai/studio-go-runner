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

* GPUs can now be aggregated for experiments needing more than 1 card, or a large card.  Uses CUDA\_VISIBLE\_DEVICES.  Validated using pytorch.
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

# 0.9.12

IMPROVEMENTS:

* support pure kubernetes based CI/CD pipeline using Ubers Makisu image builder and http://keel.sh

# 0.9.13

IMPROVEMENTS:

* Remove old style error types to drop a deprecated package, and prepare for new Go APIs

# 0.9.14

IMPROVEMENTS:

* AWS deployment example for Kubernetes
* Support for multiple secrets and services when using git-watch
* Support for standalone Kubernetes clusters as the CI platform with microk8s
* Documentation improvements for microk8s and CI

# 0.9.15

IMPROVEMENTS:

* Production container generation within CI pipeline
* Documentation improvements for microk8s and CI

# 0.9.22

IMPROVEMENTS:

* Secure coding changes
* Kubernetes based installation documentation
* Azure documentation improvements
* Nvidia bump for CUDA support of 10.0
* Go 1.11.13 support
* Improved microk8s support for image registries
* duat build tooling improvements for git-watch
* Uber Makisu image builder upgrades
* build options now import environment variables completely and NVML improvements for build
* build detects microk8s and stops after pushing the standalone build image into the microk8s cluster image registry for CI/CD offboard
* quay.io support for released images
* local git commit support for git-watcher triggering CI/CD without needing a git push to github origins
* Kubernetes 1.14 migration for CI/CD
* AWS and Azure installation scripts added for partial automation
* Azure image enhancements for the release images specific to Azure CNTK base images and AKS Azure support
* Improve file cache, worker python directories permissions masks
* Support fencing workers off from queue name matches that we do not wish to pull work from
* Treat pip install errors during experiment setup as fatal errors rather than warnings
* Updated RabbitMQ API usage
* Python 2 discontinued

FIXES:

* Catch failures during experiment process bootstrapping
* pyenv support rather then Ubuntu OS Python to improve stability
* S3 metadata related downloading was excessively and very heavy, drop for now as not yet needed

# 0.9.23

FIXES:

* Avoid persisted Azure GPU ECC errors fencing off pods, use volatile errors
* Improve unique naming strategy for pyenv
* Migrate to pyenv for testing to match production

# 0.9.24

FIXES:

* Incorporate CUDA 10 cuDNN 7.6+ as the default for Azure to avoid https://github.com/tensorflow/tensorflow/issues/24828


# 0.9.25

FIXES:

* Improve the cancel jobs on queue deletion implementation to make it more predictable


# 0.9.26

IMPROVEMENTS:

* Retry failed pip installs 3 times with a 10 second delay between retries to avoid transient network issues from abandoning tasks
* Queue servicing now long lived rather than being driven by the queue level producer function, assists with queue based cancellation
* Introduce penalty based scheduler
* Drop unused redundant support for Google PubSub
* Added a stripped down CPU single node cluster for AWS deployment example in examples/aws/cpu, complete with helloworld studioml python code.
* Message payload encryption supported across insecure transports, please see docs/message\_encryption.md for details
* Testing on microk8s rationalized and retested

FIXES:

* 281 pipdeptree related scipt had a syntax error
* 298 Kubernetes detection fixes, reinstate configuration based life cycle management


# 0.10.0

IMPROVEMENTS:

* PKI message encryption, and ed25519 message signing for messaging between python studioml clients and the go runner
* Docker Desktop support with multiple concurrent experiments on Mac and PC
* Go 1.14.4 support
* CUDA 10.1 support for all platforms except Azure
* Python 2 support retired
* Extensive improvements to the keel based build, functional and speedwise
* Quay.io is now the only offical container image registra in order that vulnerability scanning is the default for any runner related images.
* CUDA 10 Support for GPU Docker images

FIXES:

* Mount specifications for encryption were missing from the examples folder
* Titan X cards would be skipped on smaller resourced jobs, allow jobs to be run on cards more than 3 times the capacity than the job requires
* pyenv installations were failing on blank slate installs used in on-premises environments
* management requests to rabbitMQ were leaking small amounts of memory

# 0.10.1

IMPROVEMENTS:

* CUDA 10.1 support added and CUDA 8.0 support dropped
* Tensorflow 1.12 and below no longer supported
* Tensorflow 2.0 to 2.2 now supported along with pytorch 1.0.0 and above
* Migrated from Ubuntu 16.04 to 18.04

# 0.11.0

IMPROVEMENTS:

* Response queue support with encryption for RabbitMQ installations

FIXES:

* Testing improved for CI
* Individual developer workstation testing robustness improved
* Fix CWE-22 Alerts
* Workaround issues introduced for Cuda 10.1 images from Nvidia, https://github.com/NVIDIA/nvidia-docker/issues/1143

# 0.12.0

IMPROVEMENTS:

* Serving Bridge implementation with application note and complete Kubernetes deployment example

COMPATIBILITY:

* Downgrade use of S3 ListObjects to V1 to support Google Cloud Storage

# 0.12.1

IMPROVEMENTS:

* CUDA 11.0 migration
* Go 1.15.6 support with modules
* AWS Support stack refresh, with AWS MQ Managed Rabbit MQ support

# 0.13.0

IMPROVEMENTS:

* Code base pkg components used by multiple projects refactored into a new repository, github.com/leaf-ai/go-service
* Go 1.15.8 support with modules
* Remove deprecated Google Cloud storage proprietary API and use S3 mode to interact with the Google Cloud Storage offering
* S3 Credential migration to being per artifact, also environment variables are no longer used, except when the --allow-env-secrets is specified

# 0.13.1

IMPROVEMENTS:

* Go 1.16.1 support
* Docker file for the stack introduced to improve build times
* AWS MMQ support for RabbitMQ, specific instructions can be found at docs/aws_k8s.md

FIXES:

* TestTFXCfgGenerator timeout was too small causing the test to be flaky and timeout
* Prevent releases overwritting identical versions
* Fix CWE-22 code blocks for symbolic links in tarfiles, https://cwe.mitre.org/data/definitions/22.html
* CVE impacted package upgrades

# 0.13.2

IMPROVEMENTS:

* Storage limitations now used when downloading artifacts, based on the requested disk space from the StudioML client
* Idle Time limits added, new options -limit-idle-duration duration, -limit-interval duration with string values such as 10m for 10 minutes
* Jobs completed limit option added, -limit-tasks
* Document auto scaling, down to 0, in docs/aws_k8s.md, for the EKS use case.
* Go 1.16.3 support
* A100 support in non mig mode only for AWS, mixed, and single mig mode for on-premises Kubernetes

FIXES:

* Security changes made for file escape when unpacking artifact archives
* When using multiple GPUs the CUDA_VISIBLE_DEVICES was getting overwritten by the addition of new GPU devices

KNOWN BUGS:

* AWS A100 (p4d.24xlarge) mixed, and single mig support is waiting on AWS fixes

It is worth reminding that the Go module feature now being used provides module authentication using checksums against a database of modules hosted by google.  Please review the following privacy notice in regards to this feature, https://proxy.golang.org/privacy.  A vendor directory is provided as a means of avoiding Go module proxies performing integrity checking if you wish to run in a air-gaped configuration.
