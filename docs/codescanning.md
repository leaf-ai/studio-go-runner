# Static code analysis for CVE and other vulnerabilities

When used within a commercial context the studio-go-runner (runner) code is scanned using both the gosec utility, https://github.com/securego/gosec.git, and also by the Veracode Agent-Based Scan command line utility.

Installation of the Veracode utility is describe by instructions found at, https://help.veracode.com/reader/hHHR3gv0wYc2WbCclECf\_A/\_RlhBWebMi564OawIpet1w.

## gosec

The gosec utility can be downloaded using the ```GO111MODULE=on go get github.com/securego/gosec/v2/cmd/gosec``` command.  To run scans the following commands can be used:

```shell

```

Copyright © 2019-2020 Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 license.
