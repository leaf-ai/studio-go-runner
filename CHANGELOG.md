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

