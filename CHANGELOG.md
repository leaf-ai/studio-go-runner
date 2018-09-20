# 0.7.1

IMPROVEMENTS:

* Added github templates for PRs, Issues etc. ([#133](https://github.com/SentientTechnologies/studio-go-runner/issue/133)).
* Capture artifact downloading failures and insert them into the experiments output file. ([#133](https://github.com/SentientTechnologies/studio-go-runner/issue/133)).
* Faulty GPUs with bad ECC memory now caught and will only accept CPU jobs, in addition to errors being output

# 0.8.0

IMPROVEMENTS:

* Added support for testing and non-release builds using kubernetes hosted pods, please see docs/k8s.md.  Releasing from k8s hosting a future feature.

* configMap states for runner pods now support to allow pods to be idled, rolling upgrades can be done once quiesed using k8s deployment resources.

* RabbitMQ now supported within k8s testing as a seperate service within the namespace the test uses.  Please see docs/k8s.md for more information.

* Multiple test cases added, many more to go but a legitimate effort is now underway given we have k8s support and are not constrained by travis.


IMPROVEMENTS:

* Config map support within kubernentes to inform pods of desired state changes Running, Abort, Drain and suspend, Drain and terminate

