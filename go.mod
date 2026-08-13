module codeberg.org/megakuul/cloudjam

go 1.26.3

require (
	buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go v1.36.11-20260209202127-80ab13bee0bf.1
	connectrpc.com/connect v1.19.1
	connectrpc.com/validate v0.6.0
	github.com/alexedwards/argon2id v1.0.0
	github.com/aws/aws-sdk-go-v2 v1.43.5
	github.com/aws/aws-sdk-go-v2/config v1.32.36
	github.com/aws/aws-sdk-go-v2/credentials v1.19.35
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
	github.com/aws/aws-sdk-go-v2/service/sts v1.45.5
	github.com/ekristen/aws-nuke/v3 v3.66.0
	github.com/ekristen/libnuke v1.3.0
	github.com/extism/go-pdk v1.1.3
	github.com/extism/go-sdk v1.7.1
	github.com/gobwas/glob v0.2.3
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/google/uuid v1.6.0
	github.com/gruntwork-io/cloud-nuke v0.52.0
	github.com/lmittmann/tint v1.1.3
	github.com/megakuul/dynamitedb v0.6.0
	github.com/megakuul/lake v0.4.2
	github.com/spf13/cobra v1.10.2
	github.com/spf13/viper v1.21.0
	github.com/tetratelabs/wazero v1.12.0
	golang.org/x/sync v0.22.0
	google.golang.org/protobuf v1.36.12
)

require (
	atomicgo.dev/cursor v0.1.1 // indirect
	atomicgo.dev/keyboard v0.2.8 // indirect
	buf.build/go/protovalidate v1.1.3 // indirect
	cel.dev/expr v0.25.1 // indirect
	github.com/andybalholm/brotli v1.2.2 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.1 // indirect
	github.com/aws/aws-sdk-go v1.55.8 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.17 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.36 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.36 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.36 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.37 // indirect
	github.com/aws/aws-sdk-go-v2/service/accessanalyzer v1.36.12 // indirect
	github.com/aws/aws-sdk-go-v2/service/acm v1.30.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/amp v1.36.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/apigateway v1.28.12 // indirect
	github.com/aws/aws-sdk-go-v2/service/apigatewayv2 v1.24.16 // indirect
	github.com/aws/aws-sdk-go-v2/service/apprunner v1.32.14 // indirect
	github.com/aws/aws-sdk-go-v2/service/appsync v1.42.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/autoscaling v1.51.11 // indirect
	github.com/aws/aws-sdk-go-v2/service/backup v1.40.9 // indirect
	github.com/aws/aws-sdk-go-v2/service/bedrock v1.64.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/bedrockagentcorecontrol v1.14.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/cloudformation v1.65.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/cloudfront v1.45.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/cloudtrail v1.47.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/codedeploy v1.29.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/comprehend v1.40.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/configservice v1.51.11 // indirect
	github.com/aws/aws-sdk-go-v2/service/datapipeline v1.30.18 // indirect
	github.com/aws/aws-sdk-go-v2/service/datasync v1.45.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/docdb v1.41.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/docdbelastic v1.15.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/dsql v1.1.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/dynamodb v1.39.9 // indirect
	github.com/aws/aws-sdk-go-v2/service/ecr v1.40.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/ecs v1.80.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/efs v1.35.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/eks v1.74.10 // indirect
	github.com/aws/aws-sdk-go-v2/service/elasticache v1.44.11 // indirect
	github.com/aws/aws-sdk-go-v2/service/elasticbeanstalk v1.28.15 // indirect
	github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing v1.28.16 // indirect
	github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2 v1.43.11 // indirect
	github.com/aws/aws-sdk-go-v2/service/emrserverless v1.31.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/eventbridge v1.36.10 // indirect
	github.com/aws/aws-sdk-go-v2/service/firehose v1.36.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/grafana v1.26.14 // indirect
	github.com/aws/aws-sdk-go-v2/service/guardduty v1.52.9 // indirect
	github.com/aws/aws-sdk-go-v2/service/inspector2 v1.44.12 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.16 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.9.29 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/endpoint-discovery v1.10.12 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.36 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.19.37 // indirect
	github.com/aws/aws-sdk-go-v2/service/kafka v1.38.15 // indirect
	github.com/aws/aws-sdk-go-v2/service/kinesis v1.43.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/kms v1.37.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/lakeformation v1.46.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/lambda v1.88.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/macie2 v1.44.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/mgn v1.37.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/mq v1.34.19 // indirect
	github.com/aws/aws-sdk-go-v2/service/neptunegraph v1.17.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/networkfirewall v1.53.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/opensearch v1.45.10 // indirect
	github.com/aws/aws-sdk-go-v2/service/qbusiness v1.34.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/ram v1.36.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/redshift v1.53.11 // indirect
	github.com/aws/aws-sdk-go-v2/service/route53 v1.48.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/route53profiles v1.4.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/route53resolver v1.41.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/s3 v1.107.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/s3control v1.53.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/s3files v1.0.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/s3tables v1.15.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/s3vectors v1.4.10 // indirect
	github.com/aws/aws-sdk-go-v2/service/sagemaker v1.174.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/scheduler v1.12.16 // indirect
	github.com/aws/aws-sdk-go-v2/service/secretsmanager v1.34.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/securityhub v1.55.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/servicediscovery v1.39.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/ses v1.29.9 // indirect
	github.com/aws/aws-sdk-go-v2/service/shield v1.34.25 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.5.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/sns v1.33.18 // indirect
	github.com/aws/aws-sdk-go-v2/service/sqs v1.42.25 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssm v1.56.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssmquicksetup v1.3.10 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.33.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.38.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/textract v1.40.22 // indirect
	github.com/aws/aws-sdk-go-v2/service/timestreaminfluxdb v1.20.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/transfer v1.55.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/vpclattice v1.13.9 // indirect
	github.com/aws/smithy-go v1.27.7 // indirect
	github.com/benbjohnson/clock v1.3.0 // indirect
	github.com/containerd/console v1.0.3 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.6 // indirect
	github.com/dylibso/observe-sdk/go v0.0.0-20240828172851-9145d8ad07e1 // indirect
	github.com/fatih/color v1.19.0 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/fxamacker/cbor/v2 v2.9.2 // indirect
	github.com/go-errors/errors v1.4.2 // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/google/cel-go v0.27.0 // indirect
	github.com/gookit/color v1.5.0 // indirect
	github.com/gotidy/ptr v1.4.0 // indirect
	github.com/gruntwork-io/go-commons v0.17.0 // indirect
	github.com/hashicorp/errwrap v1.0.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/ianlancetaylor/demangle v0.0.0-20260724033716-83e58baca724 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/klauspost/compress v1.19.2 // indirect
	github.com/lithammer/fuzzysearch v1.1.5 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.13 // indirect
	github.com/mb0/glob v0.0.0-20160210091149-1eb79d2de6c4 // indirect
	github.com/parquet-go/bitpack v1.0.1 // indirect
	github.com/parquet-go/jsonlite v1.5.2 // indirect
	github.com/parquet-go/parquet-go v0.32.0 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/pierrec/lz4/v4 v4.1.28 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pterm/pterm v0.12.45 // indirect
	github.com/rivo/uniseg v0.2.0 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/sagikazarmark/locafero v0.11.0 // indirect
	github.com/sirupsen/logrus v1.9.4 // indirect
	github.com/sourcegraph/conc v0.3.1-0.20240121214520-5f936abd7ae8 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/stevenle/topsort v0.2.0 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/tetratelabs/wabin v0.0.0-20230304001439-f6f874872834 // indirect
	github.com/twpayne/go-geom v1.6.1 // indirect
	github.com/urfave/cli/v2 v2.10.3 // indirect
	github.com/urfave/cli/v3 v3.9.0 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	github.com/xo/terminfo v0.0.0-20210125001918-ca9a967f8778 // indirect
	github.com/xrash/smetrics v0.0.0-20201216005158-039620a65673 // indirect
	go.opentelemetry.io/proto/otlp v1.11.0 // indirect
	go.uber.org/ratelimit v0.3.1 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/crypto v0.49.0 // indirect
	golang.org/x/exp v0.0.0-20260112195511-716be5621a96 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/term v0.41.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260720211330-0afa2a65878a // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260720211330-0afa2a65878a // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
