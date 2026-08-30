//go:build wasip1

// Command aurelia-storefront is a gameday tier cloudjam challenge.
//
// Aurelia Market's storefront sits behind a real HTTP API and a WAFv2 web
// ACL that security stood up and never finished: the ACL exists, it is not
// attached to anything, one of its two rules is still in count-only mode,
// and the busiest bot pattern hitting the front door has no rule at all.
// There is no code to fix and nothing to deploy — the storefront Lambda is
// already correct and already wired. Every fix in this challenge is a WAFv2
// rule, authored or repaired, and every point is scored off real HTTP
// requests actually answered by the real API, not a resource existing.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"codeberg.org/megakuul/cloudjam/pkg/challenge"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/policy"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/apigatewayv2"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/events"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/lambda"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/logs"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/s3"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/ssm"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/wafv2"
	"github.com/google/uuid"
)

const ownerTag = "aurelia:owner"
const systemTag = "aurelia:system"
const systemName = "storefront"

const gatePrefix = "aurelia-gate-"

const (
	handoverPath  = "/aurelia/handover"
	telemetryPath = "/aurelia/telemetry"
	incidentPath  = "/aurelia/incident-report"
)

const (
	roleRetries    = 6
	roleRetryDelay = 5 * time.Second
)

const (
	minRetentionDays = 30
	maxRetentionDays = 400
)

const (
	baseRate = 15
	botRate  = 5
)

const (
	trafficPoints   = 8
	trafficRounds   = 10
	leakPoints      = -20
	leakRounds      = 8
	falsePosPoints  = -15
	falsePosRounds  = 8
	stuffPoints     = 10
	stuffRounds     = 8
	stuffLeakPoints = -25
	stuffLeakRounds = 6
	codaRounds      = 6
	codaPoints      = 12
)

var (
	photosRef   string
	archiveRef  string
	logGroupRef string

	storefrontRoleArn string
	generatorRole     string
	storefrontArn     string
	storefrontRef     string
	generatorRef      string
	scheduleRef       string

	apiRef      string
	apiId       string
	apiEndpoint string
	apiHost     string
	stageArn    string

	webAclRef string
	webAclArn string

	region string
)

var (
	stuffingStarted atomic.Bool
	codaOpen        atomic.Bool
)

func main() {
	challenge.New("Aurelia Market: Front Door Under Siege", 10*time.Second, bootstrap).
		AddDescription(
			"Aurelia Market moved its storefront behind a real HTTP API six weeks ago. "+
				"Security stood up a WAFv2 web ACL for it during the migration and then got "+
				"pulled onto something else. The ACL still exists. It is not attached to "+
				"anything. Nothing in front of the storefront has ever blocked a single "+
				"request.").
		AddDescription(
			"None of this is theoretical load. Real customers hit /catalog and /checkout "+
				"every minute. So does a scraper that has been walking /export since before "+
				"you got this ticket, and so does whatever is throwing malformed checkout "+
				"bodies at the API hoping something downstream evaluates them. All of it gets "+
				"a 200.").
		AddDescription(
			"You are the engineer who got paged. There is no architecture diagram and no "+
				"runbook — everything here carries a "+systemTag+"="+systemName+" tag, and "+
				"that tag is the entire asset register.").
		AddDescription(
			"The storefront reports what it actually served to "+telemetryPath+" once a "+
				"minute: customers served, customers wrongly blocked, and each bot pattern "+
				"blocked or leaked through. That parameter is your dashboard. You can read it. "+
				"You cannot write it.").
		AddDescription(
			"Nobody is handing you a task list. You are scored the way the storefront is "+
				"measured: on real customers served, against real bots that got through, "+
				"against real customers a clumsy rule turned away. There is more here than "+
				"there is time before this becomes an incident.").
		AddClue("where do i start",
			"The handover note is an SSM parameter at "+handoverPath+". Read it first: "+
				"aws ssm get-parameter --name "+handoverPath+".",
			-5).
		AddClue("what counts as ownership",
			"The asset register only counts resources carrying a "+ownerTag+" tag. Any "+
				"value works.",
			-15).
		AddClue("nothing is blocking anything",
			"The web ACL is not the storefront's problem — attaching it is. "+
				"aws wafv2 associate-web-acl will not make the plugin's own checks see the "+
				"attachment; it lives outside Cloud Control's view the same way every native "+
				"write does in this account. Create the attachment as its own resource: "+
				"aws cloudcontrol create-resource --type-name AWS::WAFv2::WebACLAssociation.",
			-35).
		AddClue("sqli probes are still getting a 200",
			"One rule in the web ACL already matches them. Read its Action. Count records a "+
				"match and does nothing else — it was left in testing mode and nobody came "+
				"back to flip it.",
			-25).
		AddClue("the scraper on /export has no rule at all",
			"There is exactly one rule in the ACL you didn't get for free. Write it: a "+
				"ByteMatchStatement against the request path, Action Block, added to the same "+
				"web ACL alongside the rule that's already there.",
			-30).
		AddClue("a fix that blocks everything is not a fix",
			"The storefront counts customers turned away by a rule as its own kind of "+
				"failure, priced the same as a bot getting through. A ByteMatchStatement "+
				"matched against the request path will not touch /catalog or /checkout.",
			-20).
		SetPermission(policy.Document{
			Version: policy.Version20121017,
			Statement: []policy.Statement{
				{Effect: policy.Deny, Action: policy.ActionAll, Resource: policy.ARNAll},
			},
		}).
		SetGuardrail(guardrail()).
		// --- Act I: whose account is this now ------------------------------
		AddCheck("Signed for the front door API", challenge.Check{
			Points:  20,
			Every:   20 * time.Second,
			Trigger: apiOwned,
		}).
		AddCheck("Signed for the web ACL", challenge.Check{
			Points:  20,
			Every:   20 * time.Second,
			Trigger: webAclOwned,
		}).
		AddCheck("Signed for the product photo bucket", challenge.Check{
			Points:  10,
			Every:   20 * time.Second,
			Trigger: photosOwned,
		}).
		AddCheck("Confirmed the handover", challenge.Check{
			Points:  40,
			Every:   20 * time.Second,
			Trigger: handoverConfirmed,
		}).
		AddCheck("Gave the storefront logs a retention window", challenge.Check{
			Points:  10,
			Every:   30 * time.Second,
			Trigger: logsRetained,
		}).
		// --- Act II: attach and finish the web ACL ---------------------------
		AddCheck("Attached the web ACL to the front door", challenge.Check{
			Points:  70,
			Every:   15 * time.Second,
			Trigger: webAclAssociated,
		}).
		AddCheck("Turned the sqli rule from counting to blocking", challenge.Check{
			Points:  60,
			Every:   15 * time.Second,
			Trigger: sqliRuleFixed,
		}).
		AddCheck("Wrote a rule for the /export scraper", challenge.Check{
			Points:  70,
			Every:   15 * time.Second,
			Trigger: scrapeRuleAdded,
		}).
		AddCheck("Customers are getting served", challenge.Check{
			Points:  trafficPoints,
			Every:   20 * time.Second,
			Repeat:  true,
			Trigger: bounded(trafficRounds, legitServed()),
		}).
		AddCheck("Scraper turned away", challenge.Check{
			Points:  trafficPoints,
			Every:   20 * time.Second,
			Repeat:  true,
			Trigger: bounded(trafficRounds, scrapeBlocked()),
		}).
		AddCheck("Sqli probes turned away", challenge.Check{
			Points:  trafficPoints,
			Every:   20 * time.Second,
			Repeat:  true,
			Trigger: bounded(trafficRounds, sqliBlocked()),
		}).
		AddCheck("Scraper still reaching /export", challenge.Check{
			Points:  leakPoints,
			Every:   20 * time.Second,
			Repeat:  true,
			Trigger: bounded(leakRounds, scrapeLeaking()),
		}).
		AddCheck("Sqli probes still reaching checkout", challenge.Check{
			Points:  leakPoints,
			Every:   20 * time.Second,
			Repeat:  true,
			Trigger: bounded(leakRounds, sqliLeaking()),
		}).
		AddCheck("Real customers getting blocked", challenge.Check{
			Points:  falsePosPoints,
			Every:   20 * time.Second,
			Repeat:  true,
			Trigger: bounded(falsePosRounds, legitBlocked()),
		}).
		AddEvent("credential-stuffing", challenge.Event{
			Every:   20 * time.Second,
			Trigger: frontDoorHolding(),
			Event:   theCredentialStuffing,
		}).
		AddEvent("audit-closeout", challenge.Event{
			Every:   20 * time.Second,
			Trigger: stuffingHeld,
			Event:   theAuditCloseout,
		}).
		Start()
}

// --- permissions -----------------------------------------------------------

func workingSet() policy.Actions {
	return policy.Actions{
		"s3:Get*", "s3:List*", "s3:Put*",
		"wafv2:*",
		"apigatewayv2:Get*", "apigatewayv2:List*",
		"logs:*",
		"ssm:Describe*", "ssm:Get*", "ssm:PutParameter",
		"ssm:AddTagsToResource", "ssm:ListTagsForResource",
		"events:Describe*", "events:List*",
		"lambda:GetFunction", "lambda:ListFunctions", "lambda:ListTags",
	}
}

// guardrail. The storefront and the generator are scenario-owned and never
// meant to be touched — this challenge has nothing for the player to deploy
// — so both are fenced off the same way every gameday challenge in this
// codebase fences off its own scoring machinery, and the telemetry
// parameter stays read-only.
func guardrail() policy.Document {
	return policy.Document{
		Version: policy.Version20121017,
		Statement: []policy.Statement{
			{Sid: "FrontDoor", Effect: policy.Allow, Action: workingSet(), Resource: policy.ARNAll},
			{
				Sid:    "TheStorefrontIsNotYours",
				Effect: policy.Deny,
				Action: policy.Actions{
					"lambda:UpdateFunctionCode", "lambda:UpdateFunctionConfiguration",
					"lambda:DeleteFunction", "lambda:InvokeFunction",
					"lambda:AddPermission", "lambda:RemovePermission",
				},
				Resource: policy.ARNs{policy.ARN("arn:aws:lambda:*:*:function:" + gatePrefix + "*")},
			},
			{
				Sid:      "TheScheduleIsNotYours",
				Effect:   policy.Deny,
				Action:   policy.Actions{"events:*"},
				Resource: policy.ARNs{policy.ARN("arn:aws:events:*:*:rule/" + gatePrefix + "*")},
			},
			{
				Sid:    "TheWiringIsNotYours",
				Effect: policy.Deny,
				Action: policy.Actions{
					"apigatewayv2:CreateApi", "apigatewayv2:DeleteApi", "apigatewayv2:UpdateApi",
					"apigatewayv2:CreateIntegration", "apigatewayv2:DeleteIntegration", "apigatewayv2:UpdateIntegration",
					"apigatewayv2:CreateRoute", "apigatewayv2:DeleteRoute", "apigatewayv2:UpdateRoute",
					"apigatewayv2:CreateStage", "apigatewayv2:DeleteStage", "apigatewayv2:UpdateStage",
				},
				Resource: policy.ARNAll,
			},
			{
				Sid:    "OnlyOneWebACL",
				Effect: policy.Deny,
				Action: policy.Actions{
					"wafv2:CreateWebACL", "wafv2:DeleteWebACL",
				},
				Resource: policy.ARNAll,
			},
			{
				Sid:    "TelemetryIsReadOnly",
				Effect: policy.Deny,
				Action: policy.Actions{
					"ssm:PutParameter", "ssm:DeleteParameter", "ssm:DeleteParameters",
					"ssm:LabelParameterVersion",
				},
				Resource: policy.ARNs{policy.ARN("arn:aws:ssm:*:*:parameter" + telemetryPath)},
			},
		},
	}
}

// --- AWS::WAFv2::WebACL (read shim) ------------------------------------------
//
// fakecloud serializes WebACL.Capacity as a JSON string ("0") where the real
// schema - and the generated wafv2.WebACL struct - types it as an int, which
// makes every read of the real type fail to unmarshal. This shim carries
// only the fields this challenge actually reads; encoding/json silently
// ignores the JSON keys it doesn't declare, Capacity included.
type webAcl struct {
	Arn   *string            `json:"Arn,omitempty"`
	Name  *string            `json:"Name,omitempty"`
	Tags  []wafv2.WebACLTag  `json:"Tags,omitempty"`
	Rules []wafv2.WebACLRule `json:"Rules,omitempty"`
}

func (webAcl) CloudJamType() string { return "AWS::WAFv2::WebACL" }

// --- AWS::IAM::Role ----------------------------------------------------------

type iamRole struct {
	Arn                      *string          `json:"Arn,omitempty"`
	RoleName                 *string          `json:"RoleName,omitempty"`
	Description              *string          `json:"Description,omitempty"`
	AssumeRolePolicyDocument json.RawMessage  `json:"AssumeRolePolicyDocument,omitempty"`
	Policies                 []iamRolePolicy  `json:"Policies,omitempty"`
	Tags                     []iamResourceTag `json:"Tags,omitempty"`
}

func (iamRole) CloudJamType() string { return "AWS::IAM::Role" }

type iamRolePolicy struct {
	PolicyName     *string         `json:"PolicyName,omitempty"`
	PolicyDocument json.RawMessage `json:"PolicyDocument,omitempty"`
}

type iamResourceTag struct {
	Key   *string `json:"Key,omitempty"`
	Value *string `json:"Value,omitempty"`
}

func roleArn(identifier string) string {
	if identifier == "" || strings.HasPrefix(identifier, "arn:") {
		return identifier
	}
	role, err := aws.Read[*iamRole](identifier)
	if err != nil || role == nil || role.Arn == nil || *role.Arn == "" {
		return identifier
	}
	return *role.Arn
}

func assumedBy(service string) json.RawMessage {
	return json.RawMessage(policy.Document{
		Version: policy.Version20121017,
		Statement: []policy.Statement{
			{
				Effect:    policy.Allow,
				Principal: policy.PrincipalService(service),
				Action:    policy.Actions{"sts:AssumeRole"},
			},
		},
	}.String())
}

// --- reaching fakecloud from inside its own sandbox -------------------------

const fakeCloudAccountID = "123456789012"

func localEndpointOverride() map[string]string {
	if accountFromArn(generatorRole) != fakeCloudAccountID {
		return nil
	}
	return map[string]string{
		"AWS_ENDPOINT_URL": "http://host.docker.internal:4566",
		"LOCAL_HTTP_BASE":  "http://host.docker.internal:4566",
	}
}

func accountFromArn(arn string) string {
	parts := strings.Split(arn, ":")
	if len(parts) < 5 {
		return ""
	}
	return parts[4]
}

func regionFromArn(arn string) string {
	parts := strings.Split(arn, ":")
	if len(parts) < 4 {
		return ""
	}
	return parts[3]
}

func withEnv(base, override map[string]string) map[string]string {
	merged := make(map[string]string, len(base)+len(override))
	maps.Copy(merged, base)
	maps.Copy(merged, override)
	return merged
}

// --- bootstrap ---------------------------------------------------------------

func parallel(steps ...func() error) error {
	failures := make([]error, len(steps))
	wait := sync.WaitGroup{}
	for index, step := range steps {
		wait.Go(func() { failures[index] = step() })
	}
	wait.Wait()
	return errors.Join(failures...)
}

func bootstrap(s *challenge.Scenario) error {
	run := uuid.NewString()

	if err := parallel(
		func() error { return makePhotosBucket(run) },
		func() error { return makeArchiveBucket(run) },
		func() error { return makeLogGroup(run) },
		func() error { return makeTelemetry() },
		func() error { return makeHandover() },
	); err != nil {
		return err
	}

	if err := parallel(
		func() error { return makeStorefrontRole(run) },
		func() error { return makeGeneratorRole(run) },
	); err != nil {
		return err
	}

	if err := makeStorefrontFunction(run); err != nil {
		return err
	}
	region = regionFromArn(storefrontArn)

	if err := makeApi(run); err != nil {
		return err
	}
	if err := makeIntegration(); err != nil {
		return err
	}
	if err := makeRoute(); err != nil {
		return err
	}
	if err := makeStage(); err != nil {
		return err
	}
	if err := makeApiInvokePermission(); err != nil {
		return err
	}
	if err := makeWebAcl(run); err != nil {
		return err
	}
	if err := makeGeneratorFunction(run, 0); err != nil {
		return err
	}
	if err := makeSchedule(run); err != nil {
		return err
	}
	if err := makeScheduleInvokePermission(); err != nil {
		return err
	}

	s.AddAsset("aurelia-handover.md", []byte(handoverAsset()))

	s.SetPermission(policy.Document{
		Version: policy.Version20121017,
		Statement: []policy.Statement{
			{Sid: "FrontDoor", Effect: policy.Allow, Action: workingSet(), Resource: policy.ARNAll},
		},
	})
	return nil
}

func makePhotosBucket(run string) error {
	bucket, err := aws.Create(&s3.Bucket{
		BucketName: new(fmt.Sprintf("aurelia-product-photos-%s", run)),
		PublicAccessBlockConfiguration: &s3.BucketPublicAccessBlockConfiguration{
			BlockPublicAcls:       new(false),
			BlockPublicPolicy:     new(false),
			IgnorePublicAcls:      new(false),
			RestrictPublicBuckets: new(false),
		},
		Tags: []s3.BucketTag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("product photo bucket: %w", err)
	}
	photosRef = bucket
	return nil
}

func makeArchiveBucket(run string) error {
	bucket, err := aws.Create(&s3.Bucket{
		BucketName: new(fmt.Sprintf("aurelia-returns-archive-%s", run)),
		Tags:       []s3.BucketTag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("returns archive bucket: %w", err)
	}
	archiveRef = bucket
	return nil
}

func makeLogGroup(run string) error {
	group, err := aws.Create(&logs.LogGroup{
		LogGroupName: new(fmt.Sprintf("/aurelia/storefront/%s", run)),
		Tags:         []logs.LogGroupTag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("storefront logs: %w", err)
	}
	logGroupRef = group
	return nil
}

func makeTelemetry() error {
	if _, err := aws.Create(&ssm.Parameter{
		Name:        new(telemetryPath),
		Type:        new(ssm.ParameterTypeString),
		Description: new("storefront telemetry - written by the traffic itself"),
		Value: new("window=0 legit_served=0 legit_blocked=0 scrape_blocked=0 scrape_leaked=0 " +
			"sqli_blocked=0 sqli_leaked=0 stuffing_blocked=0 stuffing_leaked=0"),
	}); err != nil {
		return fmt.Errorf("telemetry parameter: %w", err)
	}
	return nil
}

func makeHandover() error {
	if _, err := aws.Create(&ssm.Parameter{
		Name:        new(handoverPath),
		Type:        new(ssm.ParameterTypeString),
		Description: new("handover note"),
		Value:       new(handoverNote()),
	}); err != nil {
		return fmt.Errorf("handover note: %w", err)
	}
	return nil
}

func makeStorefrontRole(run string) error {
	role, err := aws.Create(&iamRole{
		RoleName:                 new(fmt.Sprintf("aurelia-storefront-%s", run)),
		Description:              new("execution role for the storefront - scenario owned"),
		AssumeRolePolicyDocument: assumedBy("lambda.amazonaws.com"),
		Policies: []iamRolePolicy{{
			PolicyName: new("storefront"),
			PolicyDocument: json.RawMessage(policy.Document{
				Version: policy.Version20121017,
				Statement: []policy.Statement{
					{
						Effect: policy.Allow,
						Action: policy.Actions{
							"logs:CreateLogGroup", "logs:CreateLogStream", "logs:PutLogEvents",
						},
						Resource: policy.ARNAll,
					},
				},
			}.String()),
		}},
		Tags: []iamResourceTag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("storefront role: %w", err)
	}
	storefrontRoleArn = roleArn(role)
	return nil
}

func makeGeneratorRole(run string) error {
	role, err := aws.Create(&iamRole{
		RoleName:                 new(fmt.Sprintf("%srole-%s", gatePrefix, run)),
		Description:              new("execution role for the storefront traffic - scenario owned"),
		AssumeRolePolicyDocument: assumedBy("lambda.amazonaws.com"),
		Policies: []iamRolePolicy{{
			PolicyName: new("traffic"),
			PolicyDocument: json.RawMessage(policy.Document{
				Version: policy.Version20121017,
				Statement: []policy.Statement{
					{
						Effect:   policy.Allow,
						Action:   policy.Actions{"ssm:GetParameter", "ssm:PutParameter"},
						Resource: policy.ARNs{policy.ARN("arn:aws:ssm:*:*:parameter" + telemetryPath)},
					},
					{
						Effect: policy.Allow,
						Action: policy.Actions{
							"logs:CreateLogGroup", "logs:CreateLogStream", "logs:PutLogEvents",
						},
						Resource: policy.ARNAll,
					},
				},
			}.String()),
		}},
		Tags: []iamResourceTag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("generator role: %w", err)
	}
	generatorRole = roleArn(role)
	return nil
}

func makeStorefrontFunction(run string) error {
	bucket, key, err := uploadLambdaCode(storefrontSource)
	if err != nil {
		return fmt.Errorf("storefront code: %w", err)
	}
	definition := &lambda.Function{
		FunctionName: new(fmt.Sprintf("%sstorefront-%s", gatePrefix, run)),
		Description:  new("aurelia market storefront - do not disable"),
		Runtime:      new("python3.12"),
		Handler:      new("index.handler"),
		Role:         new(storefrontRoleArn),
		Timeout:      new(15),
		MemorySize:   new(256),
		Code:         &lambda.Code{S3Bucket: new(bucket), S3Key: new(key)},
		Tags:         []lambda.FunctionTag{{Key: new(systemTag), Value: new(systemName)}},
	}

	for attempt := range roleRetries {
		var function string
		if function, err = aws.Create(definition); err == nil {
			storefrontRef = function
			storefrontArn = functionArn(storefrontRef)
			return nil
		}
		slog.Warn(fmt.Sprintf("storefront attempt %d/%d: %v", attempt+1, roleRetries, err))
		time.Sleep(roleRetryDelay)
	}
	return fmt.Errorf("storefront: %w", err)
}

func makeApi(run string) error {
	api, err := aws.Create(&apigatewayv2.Api{
		Name:         new(fmt.Sprintf("%sfront-door-%s", gatePrefix, run)),
		ProtocolType: new("HTTP"),
		Tags:         map[string]string{systemTag: systemName},
	})
	if err != nil {
		return fmt.Errorf("front door api: %w", err)
	}
	apiRef = api
	live, err := aws.Read[*apigatewayv2.Api](apiRef)
	if err != nil || live == nil {
		return fmt.Errorf("front door api is not readable: %w", err)
	}
	if live.ApiId != nil {
		apiId = *live.ApiId
	}
	if live.ApiEndpoint != nil {
		apiEndpoint = *live.ApiEndpoint
	}
	apiHost = strings.TrimPrefix(strings.TrimPrefix(apiEndpoint, "https://"), "http://")
	stageArn = fmt.Sprintf("arn:aws:apigateway:%s::/apis/%s/stages/$default", region, apiId)
	return nil
}

var integrationId string

func makeIntegration() error {
	integration, err := aws.Create(&apigatewayv2.Integration{
		ApiId:                new(apiId),
		IntegrationType:      new("AWS_PROXY"),
		IntegrationUri:       new(storefrontArn),
		PayloadFormatVersion: new("2.0"),
	})
	if err != nil {
		return fmt.Errorf("front door integration: %w", err)
	}
	live, err := aws.Read[*apigatewayv2.Integration](integration)
	if err != nil || live == nil || live.IntegrationId == nil {
		return fmt.Errorf("front door integration is not readable: %w", err)
	}
	integrationId = *live.IntegrationId
	return nil
}

func makeRoute() error {
	if _, err := aws.Create(&apigatewayv2.Route{
		ApiId:    new(apiId),
		RouteKey: new("$default"),
		Target:   new("integrations/" + integrationId),
	}); err != nil {
		return fmt.Errorf("front door route: %w", err)
	}
	return nil
}

func makeStage() error {
	if _, err := aws.Create(&apigatewayv2.Stage{
		ApiId:      new(apiId),
		StageName:  new("$default"),
		AutoDeploy: new(true),
	}); err != nil {
		return fmt.Errorf("front door stage: %w", err)
	}
	return nil
}

func makeApiInvokePermission() error {
	sourceArn := fmt.Sprintf("arn:aws:execute-api:%s:%s:%s/*/*", region, accountFromArn(storefrontRoleArn), apiId)
	if _, err := aws.Create(&lambda.Permission{
		FunctionName: new(storefrontRef),
		Action:       new("lambda:InvokeFunction"),
		Principal:    new("apigateway.amazonaws.com"),
		SourceArn:    new(sourceArn),
	}); err != nil {
		return fmt.Errorf("front door invoke permission: %w", err)
	}
	return nil
}

// makeWebAcl creates the web ACL exactly the way it was actually left:
// unattached, with one rule that only counts what it should be blocking, and
// no rule at all for the pattern hitting /export hardest. All three are the
// player's job, not a code fix.
func makeWebAcl(run string) error {
	acl, err := aws.Create(&wafv2.WebACL{
		Name:          new(fmt.Sprintf("%sfront-door-%s", gatePrefix, run)),
		Scope:         wafScopeRegional(),
		DefaultAction: &wafv2.DefaultAction{Allow: &wafv2.WebACLAllowAction{}},
		VisibilityConfig: &wafv2.WebACLVisibilityConfig{
			SampledRequestsEnabled:   new(true),
			CloudWatchMetricsEnabled: new(true),
			MetricName:               new("aureliaFrontDoor"),
		},
		Rules: []wafv2.WebACLRule{{
			Name:     new("flag-sqli-probe"),
			Priority: new(0),
			Action:   &wafv2.WebACLRuleAction{Count: &wafv2.WebACLCountAction{}},
			Statement: &wafv2.WebACLStatement{
				SqliMatchStatement: &wafv2.WebACLSqliMatchStatement{
					FieldToMatch: &wafv2.WebACLFieldToMatch{
						Body: &wafv2.WebACLBody{OversizeHandling: wafOversizeContinue()},
					},
					TextTransformations: []wafv2.WebACLTextTransformation{
						{Priority: new(0), Type: wafTextTransformUrlDecode()},
					},
				},
			},
			VisibilityConfig: &wafv2.WebACLVisibilityConfig{
				SampledRequestsEnabled:   new(true),
				CloudWatchMetricsEnabled: new(true),
				MetricName:               new("flagSqliProbe"),
			},
		}},
	})
	if err != nil {
		return fmt.Errorf("web acl: %w", err)
	}
	webAclRef = acl
	live, err := aws.Read[*webAcl](webAclRef)
	if err == nil && live != nil && live.Arn != nil {
		webAclArn = *live.Arn
	}
	return nil
}

func generatorEnvironment(stuffingSurge int) *lambda.Environment {
	return &lambda.Environment{Variables: withEnv(map[string]string{
		"API_HOST":               apiHost,
		"API_ENDPOINT":           apiEndpoint,
		"TALLY_PARAM":            telemetryPath,
		"BASE_RATE":              strconv.Itoa(baseRate),
		"BOT_RATE":               strconv.Itoa(botRate),
		"STUFFING_SURGE_WINDOWS": strconv.Itoa(stuffingSurge),
	}, localEndpointOverride())}
}

func makeGeneratorFunction(run string, stuffingSurge int) error {
	bucket, key, err := uploadLambdaCode(generatorSource)
	if err != nil {
		return fmt.Errorf("storefront traffic code: %w", err)
	}
	definition := &lambda.Function{
		FunctionName: new(fmt.Sprintf("%straffic-%s", gatePrefix, run)),
		Description:  new("aurelia market real customer and bot traffic - do not disable"),
		Runtime:      new("python3.12"),
		Handler:      new("index.handler"),
		Role:         new(generatorRole),
		Timeout:      new(120),
		MemorySize:   new(256),
		Code:         &lambda.Code{S3Bucket: new(bucket), S3Key: new(key)},
		Environment:  generatorEnvironment(stuffingSurge),
		Tags:         []lambda.FunctionTag{{Key: new(systemTag), Value: new(systemName)}},
	}

	for attempt := range roleRetries {
		var function string
		if function, err = aws.Create(definition); err == nil {
			generatorRef = function
			return nil
		}
		slog.Warn(fmt.Sprintf("storefront traffic attempt %d/%d: %v", attempt+1, roleRetries, err))
		time.Sleep(roleRetryDelay)
	}
	return fmt.Errorf("storefront traffic: %w", err)
}

func makeSchedule(run string) error {
	rule, err := aws.Create(&events.Rule{
		Name:               new(fmt.Sprintf("%sschedule-%s", gatePrefix, run)),
		Description:        new("sends the next window of storefront traffic, once a minute"),
		ScheduleExpression: new("rate(1 minute)"),
		State:              new(events.RuleStateENABLED),
		Targets: []events.Target{{
			Id:  new("storefront-traffic"),
			Arn: new(functionArn(generatorRef)),
		}},
	})
	if err != nil {
		return fmt.Errorf("storefront traffic schedule: %w", err)
	}
	scheduleRef = rule
	return nil
}

func makeScheduleInvokePermission() error {
	if _, err := aws.Create(&lambda.Permission{
		FunctionName: new(generatorRef),
		Action:       new("lambda:InvokeFunction"),
		Principal:    new("events.amazonaws.com"),
		SourceArn:    new(ruleArn(scheduleRef)),
	}); err != nil {
		return fmt.Errorf("storefront traffic invoke permission: %w", err)
	}
	return nil
}

func functionArn(identifier string) string {
	if identifier == "" || strings.HasPrefix(identifier, "arn:") {
		return identifier
	}
	function, err := aws.Read[*lambda.Function](identifier)
	if err != nil || function == nil || function.Arn == nil || *function.Arn == "" {
		return identifier
	}
	return *function.Arn
}

func ruleArn(identifier string) string {
	if identifier == "" || strings.HasPrefix(identifier, "arn:") {
		return identifier
	}
	rule, err := aws.Read[*events.Rule](identifier)
	if err != nil || rule == nil || rule.Arn == nil || *rule.Arn == "" {
		return identifier
	}
	return *rule.Arn
}

func handoverNote() string {
	return strings.Join([]string{
		"aurelia market front door - handover. Security started this migration and got",
		"pulled off it. Everything here is tagged " + systemTag + "=" + systemName + ".",
		"",
		"The storefront Lambda and the HTTP API in front of it are both already correct and",
		"already wired - there is nothing to deploy and nothing to fix in either. The bug is",
		"entirely in the WAFv2 web ACL sitting next to them.",
		"",
		"The web ACL exists (" + webAclArn + ") but is not attached to anything. Attaching it",
		"through aws wafv2 associate-web-acl will not make the plugin see it - that call lives",
		"outside Cloud Control's view here the same way every native write does in this",
		"account. Create the attachment as its own resource instead:",
		"  aws cloudcontrol create-resource --type-name AWS::WAFv2::WebACLAssociation \\",
		"    --desired-state '{\"ResourceArn\":\"" + stageArn + "\",\"WebACLArn\":\"" + webAclArn + "\"}'",
		"",
		"The ACL has one rule, flag-sqli-probe. Its Action is Count - it matches and does",
		"nothing. It needs to Block.",
		"",
		"The busiest bot pattern, a scraper walking /export, has no rule at all. Write one:",
		"a ByteMatchStatement against the request path, Action Block, appended to the same",
		"web ACL. A rule that blocks by path will not touch /catalog or /checkout - a rule",
		"that blocks everything is not a fix, and the storefront prices a real customer",
		"turned away the same as a bot getting through.",
		"",
		"Telemetry writes to " + telemetryPath + " every minute: customers served, customers",
		"wrongly blocked, and each bot pattern blocked or leaked through.",
	}, "\n")
}

func handoverAsset() string {
	return "# Aurelia Market — handover\n\n```\n" + handoverNote() + "\n```\n"
}

// --- Act I -------------------------------------------------------------------

func apiOwned() (bool, error) {
	api, err := readApi()
	if err != nil {
		return false, err
	}
	value, ok := api.Tags[ownerTag]
	return ok && value != "", nil
}

func webAclOwned() (bool, error) {
	acl, err := readWebAcl()
	if err != nil {
		return false, err
	}
	for _, tag := range acl.Tags {
		if tag.Key != nil && *tag.Key == ownerTag && tag.Value != nil && *tag.Value != "" {
			return true, nil
		}
	}
	return false, nil
}

func photosOwned() (bool, error) {
	bucket, err := readBucket(photosRef, "product photo bucket")
	if err != nil {
		return false, err
	}
	for _, tag := range bucket.Tags {
		if tag.Key != nil && *tag.Key == ownerTag && tag.Value != nil && *tag.Value != "" {
			return true, nil
		}
	}
	return false, nil
}

func handoverConfirmed() (bool, error) {
	value, ok, err := parameter(handoverPath)
	if err != nil || !ok {
		return false, err
	}
	return strings.TrimSpace(value) != strings.TrimSpace(handoverNote()), nil
}

func logsRetained() (bool, error) {
	group, err := readLogGroup()
	if err != nil {
		return false, err
	}
	if group.RetentionInDays == nil {
		return false, nil
	}
	days := *group.RetentionInDays
	return days >= minRetentionDays && days <= maxRetentionDays, nil
}

// --- Act II: attach and finish the web ACL ------------------------------------

func webAclAssociated() (bool, error) {
	associations, err := aws.List[*wafv2.WebACLAssociation]()
	if err != nil {
		return false, err
	}
	for identifier := range associations {
		live, err := aws.Read[*wafv2.WebACLAssociation](identifier)
		if err != nil || live == nil {
			continue
		}
		if live.ResourceArn == nil || live.WebACLArn == nil {
			continue
		}
		if strings.EqualFold(*live.ResourceArn, stageArn) && *live.WebACLArn == webAclArn {
			return true, nil
		}
	}
	return false, nil
}

func sqliRuleFixed() (bool, error) {
	acl, err := readWebAcl()
	if err != nil {
		return false, err
	}
	for _, rule := range acl.Rules {
		if rule.Statement == nil || rule.Statement.SqliMatchStatement == nil {
			continue
		}
		return rule.Action != nil && rule.Action.Block != nil, nil
	}
	return false, nil
}

// scrapeRuleAdded looks for any rule beyond the one bootstrap shipped
// (identified by not matching on sqli) that blocks outright. The player
// names and shapes this rule freely - only the effect is a contract.
func scrapeRuleAdded() (bool, error) {
	acl, err := readWebAcl()
	if err != nil {
		return false, err
	}
	for _, rule := range acl.Rules {
		if rule.Statement != nil && rule.Statement.SqliMatchStatement != nil {
			continue
		}
		if rule.Action != nil && rule.Action.Block != nil {
			return true, nil
		}
	}
	return false, nil
}

func legitServed() func() (bool, error) { return delta(func(t telemetry) int { return t.legitServed }) }
func scrapeBlocked() func() (bool, error) {
	return delta(func(t telemetry) int { return t.scrapeBlocked })
}
func sqliBlocked() func() (bool, error) { return delta(func(t telemetry) int { return t.sqliBlocked }) }
func scrapeLeaking() func() (bool, error) {
	return delta(func(t telemetry) int { return t.scrapeLeaked })
}
func sqliLeaking() func() (bool, error) { return delta(func(t telemetry) int { return t.sqliLeaked }) }
func legitBlocked() func() (bool, error) {
	return delta(func(t telemetry) int { return t.legitBlocked })
}
func stuffingBlocked() func() (bool, error) {
	return delta(func(t telemetry) int { return t.stuffingBlocked })
}
func stuffingLeaking() func() (bool, error) {
	return delta(func(t telemetry) int { return t.stuffingLeaked })
}

func delta(field func(telemetry) int) func() (bool, error) {
	last := -1
	return func() (bool, error) {
		state, ok, err := readTelemetry()
		if err != nil || !ok {
			return false, err
		}
		now := field(state)
		if last < 0 {
			last = now
			return false, nil
		}
		rose := now > last
		last = now
		return rose, nil
	}
}

// frontDoorHolding fires the credential-stuffing escalation once the front
// door has proven it can serve customers while turning away both existing
// bot patterns in the same window.
func frontDoorHolding() func() (bool, error) {
	return func() (bool, error) {
		state, ok, err := readTelemetry()
		if err != nil || !ok {
			return false, err
		}
		return state.legitServed > 0 && state.scrapeBlocked > 0 && state.sqliBlocked > 0, nil
	}
}

// --- Act III: credential stuffing ---------------------------------------------

func theCredentialStuffing(ctx context.Context, s *challenge.Scenario) error {
	if !stuffingStarted.CompareAndSwap(false, true) {
		return nil
	}

	s.AddDescription(
		"The front door is holding. Within the hour something starts hammering /login, " +
			"reusing the same session identifier across dozens of requests a minute — " +
			"credential stuffing, not browsing. Neither rule you've written catches it: " +
			"it isn't a body pattern and it isn't a fixed path.")
	s.AddClue("the same header keeps showing up",
		"A RateBasedStatement can key on more than the caller's address. Its CustomKeys can "+
			"aggregate on a specific request header instead — the same X-Session-Id value "+
			"repeating past a limit inside a short window is exactly what stuffing looks "+
			"like and browsing never does.",
		-30)
	s.AddClue("how tight a window",
		"A limit low enough to catch dozens of requests reusing one header value inside a "+
			"minute or two will not touch a customer who only ever sends one.",
		-20)

	s.AddCheck("Wrote a rate rule for the stuffing pattern", challenge.Check{
		Points:  70,
		Every:   15 * time.Second,
		Trigger: rateRuleAdded,
	})
	s.AddCheck("Stuffing burst turned away", challenge.Check{
		Points:  stuffPoints,
		Every:   20 * time.Second,
		Repeat:  true,
		Trigger: bounded(stuffRounds, stuffingBlocked()),
	})
	s.AddCheck("Stuffing burst still getting through", challenge.Check{
		Points:  stuffLeakPoints,
		Every:   20 * time.Second,
		Repeat:  true,
		Trigger: bounded(stuffLeakRounds, stuffingLeaking()),
	})

	if generatorRef == "" {
		return fmt.Errorf("storefront traffic was never provisioned")
	}
	if err := aws.Update(generatorRef, &lambda.Function{
		Environment: generatorEnvironment(1),
	}); err != nil {
		return fmt.Errorf("switch storefront traffic to stuffing surge: %w", err)
	}
	slog.Info("credential stuffing surge enabled on storefront traffic")
	return nil
}

func rateRuleAdded() (bool, error) {
	acl, err := readWebAcl()
	if err != nil {
		return false, err
	}
	for _, rule := range acl.Rules {
		if rule.Statement == nil || rule.Statement.RateBasedStatement == nil {
			continue
		}
		if rule.Action != nil && rule.Action.Block != nil {
			return true, nil
		}
	}
	return false, nil
}

func stuffingHeld() (bool, error) {
	if !stuffingStarted.Load() {
		return false, nil
	}
	added, err := rateRuleAdded()
	if err != nil || !added {
		return false, err
	}
	state, ok, err := readTelemetry()
	if err != nil || !ok {
		return false, err
	}
	return state.stuffingBlocked > 0, nil
}

// --- the coda: audit closeout --------------------------------------------------

func theAuditCloseout(ctx context.Context, s *challenge.Scenario) error {
	if !codaOpen.CompareAndSwap(false, true) {
		return nil
	}

	s.AddDescription(
		"The front door holds against everything it has seen so far. An auditor wants proof " +
			"it will keep holding: WAF logging turned on, the returns archive taken off the " +
			"open internet, and a short incident report for what actually happened.")
	s.AddClue("waf logging has one gotcha",
		"AWS::WAFv2::LoggingConfiguration will accept a CloudWatch Logs group as a "+
			"destination, but only one whose name starts with aws-waf-logs-. Anything else "+
			"is silently invalid.",
		-25)
	s.AddClue("what the report needs to quote",
		"Put the incident report in "+incidentPath+". It has to quote two things: the arn "+
			"of the web acl, and the arn of the log group WAF logging now writes to.",
		-20)

	s.AddCheck("Turned on WAF logging", challenge.Check{
		Points:  55,
		Every:   20 * time.Second,
		Trigger: wafLoggingEnabled,
	})
	s.AddCheck("Put the returns archive under a company key", challenge.Check{
		Points:  40,
		Every:   20 * time.Second,
		Trigger: archiveEncrypted,
	})
	s.AddCheck("Took the returns archive off the public internet", challenge.Check{
		Points:  40,
		Every:   20 * time.Second,
		Trigger: archiveClosed,
	})
	s.AddCheck("Turned on versioning for the returns archive", challenge.Check{
		Points:  25,
		Every:   20 * time.Second,
		Trigger: archiveVersioned,
	})
	s.AddCheck("Filed the incident report", challenge.Check{
		Points:  50,
		Every:   20 * time.Second,
		Trigger: incidentReportFiled,
	})
	s.AddCheck("Kept the front door clean", challenge.Check{
		Points:  codaPoints,
		Every:   30 * time.Second,
		Repeat:  true,
		Trigger: bounded(codaRounds, frontDoorClean()),
	})

	return nil
}

func frontDoorClean() func() (bool, error) {
	lastScrapeLeak, lastSqliLeak, lastStuffLeak, lastServed := -1, -1, -1, -1
	return func() (bool, error) {
		state, ok, err := readTelemetry()
		if err != nil || !ok {
			return false, err
		}
		if lastServed < 0 {
			lastServed = state.legitServed
			lastScrapeLeak, lastSqliLeak, lastStuffLeak = state.scrapeLeaked, state.sqliLeaked, state.stuffingLeaked
			return false, nil
		}
		served := state.legitServed - lastServed
		leaked := (state.scrapeLeaked - lastScrapeLeak) + (state.sqliLeaked - lastSqliLeak) + (state.stuffingLeaked - lastStuffLeak)
		lastServed = state.legitServed
		lastScrapeLeak, lastSqliLeak, lastStuffLeak = state.scrapeLeaked, state.sqliLeaked, state.stuffingLeaked
		return served > 0 && leaked <= 0, nil
	}
}

func wafLoggingEnabled() (bool, error) {
	configs, err := aws.List[*wafv2.LoggingConfiguration]()
	if err != nil {
		return false, err
	}
	for identifier := range configs {
		live, err := aws.Read[*wafv2.LoggingConfiguration](identifier)
		if err != nil || live == nil {
			continue
		}
		if live.ResourceArn == nil || *live.ResourceArn != webAclArn {
			continue
		}
		for _, destination := range live.LogDestinationConfigs {
			if strings.Contains(destination, ":log-group:aws-waf-logs-") {
				return true, nil
			}
		}
	}
	return false, nil
}

func archiveEncrypted() (bool, error) {
	bucket, err := readBucket(archiveRef, "returns archive")
	if err != nil {
		return false, err
	}
	if bucket.BucketEncryption == nil {
		return false, nil
	}
	for _, rule := range bucket.BucketEncryption.ServerSideEncryptionConfiguration {
		if rule.ServerSideEncryptionByDefault == nil {
			continue
		}
		algorithm := rule.ServerSideEncryptionByDefault.SSEAlgorithm
		if algorithm != nil && *algorithm == s3.ServerSideEncryptionByDefaultSSEAlgorithmAwsKms {
			return true, nil
		}
	}
	return false, nil
}

func archiveClosed() (bool, error) {
	bucket, err := readBucket(archiveRef, "returns archive")
	if err != nil {
		return false, err
	}
	block := bucket.PublicAccessBlockConfiguration
	if block == nil {
		return false, nil
	}
	for _, flag := range []*bool{
		block.BlockPublicAcls, block.BlockPublicPolicy,
		block.IgnorePublicAcls, block.RestrictPublicBuckets,
	} {
		if flag == nil || !*flag {
			return false, nil
		}
	}
	return true, nil
}

func archiveVersioned() (bool, error) {
	bucket, err := readBucket(archiveRef, "returns archive")
	if err != nil {
		return false, err
	}
	if bucket.VersioningConfiguration == nil || bucket.VersioningConfiguration.Status == nil {
		return false, nil
	}
	return *bucket.VersioningConfiguration.Status == s3.VersioningConfigurationStatusEnabled, nil
}

func incidentReportFiled() (bool, error) {
	value, ok, err := parameter(incidentPath)
	if err != nil || !ok {
		return false, err
	}
	if webAclArn == "" || !strings.Contains(value, webAclArn) {
		return false, nil
	}
	configs, err := aws.List[*wafv2.LoggingConfiguration]()
	if err != nil {
		return false, err
	}
	for identifier := range configs {
		live, err := aws.Read[*wafv2.LoggingConfiguration](identifier)
		if err != nil || live == nil {
			continue
		}
		if live.ResourceArn == nil || *live.ResourceArn != webAclArn {
			continue
		}
		for _, destination := range live.LogDestinationConfigs {
			if destination != "" && strings.Contains(value, destination) {
				return true, nil
			}
		}
	}
	return false, nil
}

func bounded(rounds int, trigger func() (bool, error)) func() (bool, error) {
	fired := 0
	return func() (bool, error) {
		if fired >= rounds {
			return false, nil
		}
		passed, err := trigger()
		if err != nil || !passed {
			return false, err
		}
		fired++
		return true, nil
	}
}

// --- reads ---------------------------------------------------------------------

func readApi() (*apigatewayv2.Api, error) {
	if apiRef == "" {
		return nil, fmt.Errorf("front door api was never provisioned")
	}
	api, err := aws.Read[*apigatewayv2.Api](apiRef)
	if err != nil {
		return nil, err
	}
	if api == nil {
		return nil, fmt.Errorf("front door api is not readable")
	}
	return api, nil
}

func readWebAcl() (*webAcl, error) {
	if webAclRef == "" {
		return nil, fmt.Errorf("web acl was never provisioned")
	}
	acl, err := aws.Read[*webAcl](webAclRef)
	if err != nil {
		return nil, err
	}
	if acl == nil {
		return nil, fmt.Errorf("web acl is not readable")
	}
	return acl, nil
}

func readBucket(ref, label string) (*s3.Bucket, error) {
	if ref == "" {
		return nil, fmt.Errorf("%s was never provisioned", label)
	}
	bucket, err := aws.Read[*s3.Bucket](ref)
	if err != nil {
		return nil, err
	}
	if bucket == nil {
		return nil, fmt.Errorf("%s is not readable", label)
	}
	return bucket, nil
}

func readLogGroup() (*logs.LogGroup, error) {
	if logGroupRef == "" {
		return nil, fmt.Errorf("storefront logs were never provisioned")
	}
	group, err := aws.Read[*logs.LogGroup](logGroupRef)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, fmt.Errorf("storefront logs are not readable")
	}
	return group, nil
}

type telemetry struct {
	window          int
	legitServed     int
	legitBlocked    int
	scrapeBlocked   int
	scrapeLeaked    int
	sqliBlocked     int
	sqliLeaked      int
	stuffingBlocked int
	stuffingLeaked  int
}

func readTelemetry() (telemetry, bool, error) {
	value, ok, err := parameter(telemetryPath)
	if err != nil || !ok {
		return telemetry{}, false, err
	}
	state := telemetry{}
	for field := range strings.FieldsSeq(value) {
		key, raw, found := strings.Cut(field, "=")
		if !found {
			continue
		}
		number, err := strconv.Atoi(raw)
		if err != nil {
			continue
		}
		switch key {
		case "window":
			state.window = number
		case "legit_served":
			state.legitServed = number
		case "legit_blocked":
			state.legitBlocked = number
		case "scrape_blocked":
			state.scrapeBlocked = number
		case "scrape_leaked":
			state.scrapeLeaked = number
		case "sqli_blocked":
			state.sqliBlocked = number
		case "sqli_leaked":
			state.sqliLeaked = number
		case "stuffing_blocked":
			state.stuffingBlocked = number
		case "stuffing_leaked":
			state.stuffingLeaked = number
		}
	}
	return state, true, nil
}

func parameter(path string) (string, bool, error) {
	parameters, err := aws.List[*ssm.Parameter]()
	if err != nil {
		return "", false, err
	}
	found, ok := parameters[path]
	if !ok {
		return "", false, nil
	}
	if found != nil && found.Value != nil && *found.Value != "" {
		return *found.Value, true, nil
	}
	live, err := aws.Read[*ssm.Parameter](path)
	if err != nil {
		return "", false, err
	}
	if live == nil || live.Value == nil {
		return "", false, nil
	}
	return *live.Value, true, nil
}

// --- wafv2 enum helpers -----------------------------------------------------
//
// These exist only because the generated wafv2 package types every enum as
// its own named string type, so a bare string literal does not assign
// directly - a small tax for the same generated-from-schema safety every
// other service package in this codebase has.

func wafScopeRegional() *wafv2.WebACLScope {
	scope := wafv2.WebACLScopeREGIONAL
	return &scope
}

func wafOversizeContinue() *wafv2.WebACLOversizeHandling {
	handling := wafv2.WebACLOversizeHandling("CONTINUE")
	return &handling
}

func wafTextTransformUrlDecode() *wafv2.WebACLTextTransformationType {
	transform := wafv2.WebACLTextTransformationType("URL_DECODE")
	return &transform
}
