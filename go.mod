module codeberg.org/megakuul/cloudjam

go 1.26.3

require (
	buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go v1.36.11-20260209202127-80ab13bee0bf.1
	connectrpc.com/connect v1.19.1
	connectrpc.com/validate v0.6.0
	github.com/alexedwards/argon2id v1.0.0
	github.com/aws/aws-sdk-go-v2 v1.43.0
	github.com/aws/aws-sdk-go-v2/config v1.32.31
	github.com/aws/aws-sdk-go-v2/credentials v1.19.30
	github.com/aws/aws-sdk-go-v2/service/acmpca v1.47.8
	github.com/aws/aws-sdk-go-v2/service/cloudcontrol v1.30.5
	github.com/aws/aws-sdk-go-v2/service/cloudhsmv2 v1.35.6
	github.com/aws/aws-sdk-go-v2/service/cloudwatch v1.61.1
	github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs v1.78.2
	github.com/aws/aws-sdk-go-v2/service/costexplorer v1.65.3
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.311.0
	github.com/aws/aws-sdk-go-v2/service/iam v1.54.7
	github.com/aws/aws-sdk-go-v2/service/organizations v1.51.12
	github.com/aws/aws-sdk-go-v2/service/rds v1.119.5
	github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi v1.33.5
	github.com/aws/aws-sdk-go-v2/service/sts v1.45.0
	github.com/gobwas/glob v0.2.3
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/google/uuid v1.6.0
	github.com/gruntwork-io/cloud-nuke v0.52.0
	github.com/lmittmann/tint v1.1.3
	github.com/megakuul/dynamitedb v0.2.6
	github.com/megakuul/lake v0.4.1
	github.com/spf13/cobra v1.10.2
	github.com/spf13/viper v1.21.0
	golang.org/x/sync v0.20.0
	google.golang.org/protobuf v1.36.11
)

require (
	atomicgo.dev/cursor v0.1.1 // indirect
	atomicgo.dev/keyboard v0.2.8 // indirect
	buf.build/go/protovalidate v1.1.3 // indirect
	cel.dev/expr v0.25.1 // indirect
	github.com/andybalholm/brotli v1.2.2 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.1 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.14 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.31 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.31 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.31 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.32 // indirect
	github.com/aws/aws-sdk-go-v2/service/accessanalyzer v1.36.12 // indirect
	github.com/aws/aws-sdk-go-v2/service/acm v1.30.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/amp v1.31.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/apigateway v1.28.10 // indirect
	github.com/aws/aws-sdk-go-v2/service/apigatewayv2 v1.24.16 // indirect
	github.com/aws/aws-sdk-go-v2/service/apprunner v1.32.14 // indirect
	github.com/aws/aws-sdk-go-v2/service/autoscaling v1.51.11 // indirect
	github.com/aws/aws-sdk-go-v2/service/backup v1.40.9 // indirect
	github.com/aws/aws-sdk-go-v2/service/cloudformation v1.65.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/cloudfront v1.45.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/cloudtrail v1.47.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/codedeploy v1.29.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/configservice v1.51.11 // indirect
	github.com/aws/aws-sdk-go-v2/service/datapipeline v1.30.18 // indirect
	github.com/aws/aws-sdk-go-v2/service/datasync v1.45.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/dynamodb v1.39.9 // indirect
	github.com/aws/aws-sdk-go-v2/service/ecr v1.40.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/ecs v1.53.12 // indirect
	github.com/aws/aws-sdk-go-v2/service/efs v1.34.10 // indirect
	github.com/aws/aws-sdk-go-v2/service/eks v1.57.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/elasticache v1.44.11 // indirect
	github.com/aws/aws-sdk-go-v2/service/elasticbeanstalk v1.28.15 // indirect
	github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing v1.28.16 // indirect
	github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2 v1.43.11 // indirect
	github.com/aws/aws-sdk-go-v2/service/eventbridge v1.36.10 // indirect
	github.com/aws/aws-sdk-go-v2/service/firehose v1.36.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/grafana v1.26.14 // indirect
	github.com/aws/aws-sdk-go-v2/service/guardduty v1.52.9 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.13 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.9.24 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/endpoint-discovery v1.10.12 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.31 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.19.32 // indirect
	github.com/aws/aws-sdk-go-v2/service/kafka v1.38.15 // indirect
	github.com/aws/aws-sdk-go-v2/service/kinesis v1.43.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/kms v1.37.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/lambda v1.88.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/macie2 v1.44.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/mq v1.34.19 // indirect
	github.com/aws/aws-sdk-go-v2/service/networkfirewall v1.44.13 // indirect
	github.com/aws/aws-sdk-go-v2/service/opensearch v1.45.10 // indirect
	github.com/aws/aws-sdk-go-v2/service/ram v1.36.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/redshift v1.53.11 // indirect
	github.com/aws/aws-sdk-go-v2/service/route53 v1.48.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/s3 v1.106.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/s3control v1.53.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/sagemaker v1.174.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/scheduler v1.12.16 // indirect
	github.com/aws/aws-sdk-go-v2/service/secretsmanager v1.34.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/securityhub v1.55.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/servicediscovery v1.39.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/ses v1.29.9 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.5.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/sns v1.33.18 // indirect
	github.com/aws/aws-sdk-go-v2/service/sqs v1.37.13 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssm v1.56.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.33.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.38.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/vpclattice v1.13.9 // indirect
	github.com/aws/smithy-go v1.27.4 // indirect
	github.com/containerd/console v1.0.3 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.6 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/go-errors/errors v1.4.2 // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/google/cel-go v0.27.0 // indirect
	github.com/gookit/color v1.5.0 // indirect
	github.com/gruntwork-io/go-commons v0.17.0 // indirect
	github.com/hashicorp/errwrap v1.0.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/klauspost/compress v1.19.1 // indirect
	github.com/lithammer/fuzzysearch v1.1.5 // indirect
	github.com/mattn/go-runewidth v0.0.13 // indirect
	github.com/parquet-go/bitpack v1.0.0 // indirect
	github.com/parquet-go/jsonlite v1.5.2 // indirect
	github.com/parquet-go/parquet-go v0.30.2-0.20260721183652-ef5d53accfc9 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/pierrec/lz4/v4 v4.1.27 // indirect
	github.com/pterm/pterm v0.12.45 // indirect
	github.com/rivo/uniseg v0.2.0 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/sagikazarmark/locafero v0.11.0 // indirect
	github.com/sirupsen/logrus v1.8.3 // indirect
	github.com/sourcegraph/conc v0.3.1-0.20240121214520-5f936abd7ae8 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/twpayne/go-geom v1.6.1 // indirect
	github.com/urfave/cli/v2 v2.10.3 // indirect
	github.com/xo/terminfo v0.0.0-20210125001918-ca9a967f8778 // indirect
	github.com/xrash/smetrics v0.0.0-20201216005158-039620a65673 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/crypto v0.49.0 // indirect
	golang.org/x/exp v0.0.0-20260112195511-716be5621a96 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/term v0.41.0 // indirect
	golang.org/x/text v0.35.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260316180232-0b37fe3546d5 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260316180232-0b37fe3546d5 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)
