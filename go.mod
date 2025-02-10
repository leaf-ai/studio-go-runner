module github.com/leaf-ai/studio-go-runner

go 1.23.3

require (
	github.com/BurntSushi/toml v1.4.0
	github.com/Masterminds/sprig/v3 v3.3.0
	github.com/aws/aws-sdk-go v1.55.6
	github.com/aws/aws-sdk-go-v2 v1.36.0
	github.com/aws/aws-sdk-go-v2/config v1.29.4
	github.com/aws/aws-sdk-go-v2/credentials v1.17.57
	github.com/aws/aws-sdk-go-v2/service/s3 v1.75.2
	github.com/aws/aws-sdk-go-v2/service/sqs v1.37.12
	github.com/davecgh/go-spew v1.1.1
	github.com/dgryski/go-farm v0.0.0-20240924180020-3414d57e47da
	github.com/dustin/go-humanize v1.0.1
	github.com/evanphx/json-patch v0.5.2
	github.com/go-stack/stack v1.8.1
	github.com/go-test/deep v1.1.1
	github.com/go-yaml/yaml v2.1.0+incompatible
	github.com/jjeffery/kv v0.8.1
	github.com/karlmutch/base62 v0.0.0-20150408093626-b80cdc656a7a
	github.com/karlmutch/ccache v2.0.2+incompatible
	github.com/karlmutch/circbuf v0.0.0-20150827004946-bbbad097214e
	github.com/karlmutch/envflag v0.0.0-20211229205350-9dc3b5cc21e3
	github.com/karlmutch/go-cache v2.0.0+incompatible
	github.com/karlmutch/hashstructure v0.0.0-20170609045927-2bca23e0e452
	github.com/karlmutch/logxi v0.0.0-20220617052525-10dee3b1fe0c
	github.com/karlmutch/vtclean v0.0.0-20170504063817-d14193dfc626
	github.com/leaf-ai/go-service v0.0.4
	github.com/lthibault/jitterbug v2.0.0+incompatible
	github.com/mitchellh/copystructure v1.2.0
	github.com/pkg/errors v0.9.1
	github.com/rs/xid v1.6.0
	github.com/shirou/gopsutil v3.21.11+incompatible
	github.com/tebeka/atexit v0.3.0
	go.uber.org/atomic v1.11.0
)

require (
	dario.cat/mergo v1.0.1 // indirect
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Masterminds/semver/v3 v3.3.0 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.6.8 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.16.27 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.3.31 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.6.31 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.2 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.3.31 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.12.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.5.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.12.12 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.18.12 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.24.14 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.28.13 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.33.12 // indirect
	github.com/aws/smithy-go v1.22.2 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/huandu/xstrings v1.5.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/karlseguin/expect v1.0.8 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/shopspring/decimal v1.4.0 // indirect
	github.com/spf13/cast v1.7.0 // indirect
	github.com/tklauser/go-sysconf v0.3.14 // indirect
	github.com/tklauser/numcpus v0.8.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	golang.org/x/crypto v0.32.0 // indirect
	golang.org/x/sys v0.29.0 // indirect
)

replace google.golang.org/protobuf => google.golang.org/protobuf v1.33.0
