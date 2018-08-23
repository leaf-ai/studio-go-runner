# 0.7.1

IMPROVEMENTS:

* Added github templates for PRs, Issues etc. ([#133](https://github.com/SentientTechnologies/studio-go-runner/issue/133)).
* Capture artifact downloading failures and insert them into the experiments output file. ([#133](https://github.com/SentientTechnologies/studio-go-runner/issue/133)).
* Faulty GPUs with bad ECC memory now caught and will only accept CPU jobs, in addition to errors being output

# 0.8.0

IMPROVEMENTS:

* Config map support within kubernentes to inform pods of desired state changes Running, Abort, Drain and suspend, Drain and terminate

