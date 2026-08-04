//go:build !wasip1

// The tests exercise the SDK against a fake host, which is only meaningful off
// wasip1 — there the transport is the real extism import and there is no host to
// fake.

package aws

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"codeberg.org/megakuul/cloudjam/pkg/challenge/api"
	"github.com/awslabs/goformation/v7/cloudformation/ec2"
	"github.com/awslabs/goformation/v7/cloudformation/s3"
)

// fakeHost records what the SDK sends and replays canned state.
type fakeHost struct {
	noHost // anything not overridden fails loudly

	created   api.CreateResourceInput
	updated   api.UpdateResourceInput
	deleted   api.DeleteResourceInput
	meta      api.InitInput
	scores    []api.UpdateScoreInput
	reported  []string
	state     string
	listing   map[string]string
	readErr   error
	createErr error
}

func (f *fakeHost) Init(in api.InitInput) (api.InitOutput, error) {
	f.meta = in
	return api.InitOutput{}, nil
}

func (f *fakeHost) Report(in api.ReportInput) (api.ReportOutput, error) {
	f.reported = append(f.reported, in.Error)
	return api.ReportOutput{}, nil
}

func (f *fakeHost) UpdateScore(in api.UpdateScoreInput) (api.UpdateScoreOutput, error) {
	f.scores = append(f.scores, in)
	return api.UpdateScoreOutput{}, nil
}

func (f *fakeHost) CreateResource(in api.CreateResourceInput) (api.CreateResourceOutput, error) {
	if f.createErr != nil {
		return api.CreateResourceOutput{}, f.createErr
	}
	f.created = in
	return api.CreateResourceOutput{}, nil
}

func (f *fakeHost) ReadResource(api.ReadResourceInput) (api.ReadResourceOutput, error) {
	if f.readErr != nil {
		return api.ReadResourceOutput{}, f.readErr
	}
	return api.ReadResourceOutput{State: f.state}, nil
}

func (f *fakeHost) UpdateResource(in api.UpdateResourceInput) (api.UpdateResourceOutput, error) {
	f.updated = in
	return api.UpdateResourceOutput{}, nil
}

func (f *fakeHost) DeleteResource(in api.DeleteResourceInput) (api.DeleteResourceOutput, error) {
	f.deleted = in
	return api.DeleteResourceOutput{}, nil
}

func (f *fakeHost) ListResource(api.ListResourceInput) (api.ListResourceOutput, error) {
	return api.ListResourceOutput{Resources: f.listing}, nil
}

func (f *fakeHost) Log(string) {}

// swap installs f as the host for the duration of the test.
func swap(t *testing.T, f *fakeHost) {
	t.Helper()
	previous := host
	host = f
	t.Cleanup(func() { host = previous })
}

func TestTypeName(t *testing.T) {
	if got := TypeName[*s3.Bucket](); got != "AWS::S3::Bucket" {
		t.Errorf("TypeName[*s3.Bucket]() = %q, want AWS::S3::Bucket", got)
	}
	if got := TypeName[*ec2.VPC](); got != "AWS::EC2::VPC" {
		t.Errorf("TypeName[*ec2.VPC]() = %q, want AWS::EC2::VPC", got)
	}
}

func TestCreateSendsBarePropertiesAndWait(t *testing.T) {
	f := &fakeHost{}
	swap(t, f)

	if err := Create(&s3.Bucket{BucketName: String("cloudjam-demo")}, Wait); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if f.created.Type != "AWS::S3::Bucket" {
		t.Errorf("type = %q, want AWS::S3::Bucket", f.created.Type)
	}
	if !f.created.Wait {
		t.Error("aws.Wait did not set the wait flag")
	}
	// Cloud Control wants the bare properties, not goformation's
	// {"Type":...,"Properties":...} template fragment.
	if f.created.Desired != `{"BucketName":"cloudjam-demo"}` {
		t.Errorf("desired = %s", f.created.Desired)
	}
}

func TestCreateDefaultsToNotWaiting(t *testing.T) {
	f := &fakeHost{}
	swap(t, f)

	if err := Create(&s3.Bucket{}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if f.created.Wait {
		t.Error("wait should be off unless aws.Wait is passed")
	}
	if f.created.Desired != "{}" {
		t.Errorf("empty resource desired = %s, want {}", f.created.Desired)
	}
}

func TestReadIgnoresReadOnlyAttributes(t *testing.T) {
	f := &fakeHost{state: `{
		"BucketName": "cloudjam-demo",
		"Arn": "arn:aws:s3:::cloudjam-demo",
		"DualStackDomainName": "cloudjam-demo.s3.dualstack.eu-central-1.amazonaws.com",
		"BucketEncryption": {
			"ServerSideEncryptionConfiguration": [
				{"ServerSideEncryptionByDefault": {"SSEAlgorithm": "AES256"}}
			]
		}
	}`}
	swap(t, f)

	bucket, err := Read[*s3.Bucket]("cloudjam-demo")
	if err != nil {
		// goformation decodes with DisallowUnknownFields, so an unfiltered
		// decode would fail on Arn.
		t.Fatalf("Read: %v", err)
	}
	if Value(bucket.BucketName) != "cloudjam-demo" {
		t.Errorf("BucketName = %v", bucket.BucketName)
	}
	if bucket.BucketEncryption == nil {
		t.Fatal("BucketEncryption not decoded")
	}
	rules := bucket.BucketEncryption.ServerSideEncryptionConfiguration
	if len(rules) != 1 || rules[0].ServerSideEncryptionByDefault.SSEAlgorithm != "AES256" {
		t.Errorf("encryption rules = %+v", rules)
	}
}

func TestReadStateExposesReadOnlyAttributes(t *testing.T) {
	f := &fakeHost{state: `{"CidrBlock":"10.0.0.0/16","VpcId":"vpc-123","EnableDnsSupport":true}`}
	swap(t, f)

	// ec2.VPC has no VpcId field at all: it is a read-only attribute.
	state, err := ReadState[*ec2.VPC]("vpc-123")
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if Value(state.Properties.CidrBlock) != "10.0.0.0/16" {
		t.Errorf("CidrBlock = %v", state.Properties.CidrBlock)
	}
	id, ok := state.Attr("VpcId")
	if !ok || id != "vpc-123" {
		t.Errorf("Attr(VpcId) = %q, %v", id, ok)
	}
	if _, ok := state.Attr("Nope"); ok {
		t.Error("Attr returned ok for a missing attribute")
	}
}

func TestReadPropagatesHostError(t *testing.T) {
	swap(t, &fakeHost{readErr: errors.New("boom")})

	if _, err := Read[*s3.Bucket]("x"); err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdateEncodesPatch(t *testing.T) {
	f := &fakeHost{}
	swap(t, f)

	err := Update[*s3.Bucket]("cloudjam-demo", Patch{
		Replace("/VersioningConfiguration/Status", "Enabled"),
		Remove("/Tags/0"),
		Add("/PublicAccessBlockConfiguration/BlockPublicAcls", false),
	}, Wait)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	if f.updated.Type != "AWS::S3::Bucket" || f.updated.Identifier != "cloudjam-demo" {
		t.Errorf("target = %s %s", f.updated.Type, f.updated.Identifier)
	}
	if !f.updated.Wait {
		t.Error("wait flag not forwarded")
	}
	const want = `[{"op":"replace","path":"/VersioningConfiguration/Status","value":"Enabled"},` +
		`{"op":"remove","path":"/Tags/0"},` +
		`{"op":"add","path":"/PublicAccessBlockConfiguration/BlockPublicAcls","value":false}]`
	if f.updated.Patch != want {
		t.Errorf("patch =\n%s\nwant\n%s", f.updated.Patch, want)
	}
}

func TestUpdateRejectsEmptyPatch(t *testing.T) {
	swap(t, &fakeHost{})
	if err := Update[*s3.Bucket]("x", nil); err == nil {
		t.Fatal("expected an error for an empty patch")
	}
}

func TestDelete(t *testing.T) {
	f := &fakeHost{}
	swap(t, f)

	if err := Delete[*ec2.VPC]("vpc-123", Wait); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if f.deleted.Type != "AWS::EC2::VPC" || f.deleted.Identifier != "vpc-123" || !f.deleted.Wait {
		t.Errorf("delete input = %+v", f.deleted)
	}
}

func TestListKeysByIdentifier(t *testing.T) {
	swap(t, &fakeHost{listing: map[string]string{
		"vpc-1": `{"CidrBlock":"10.0.0.0/16","VpcId":"vpc-1"}`,
		"vpc-2": `{"CidrBlock":"10.1.0.0/16","VpcId":"vpc-2"}`,
	}})

	vpcs, err := List[*ec2.VPC]()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(vpcs) != 2 {
		t.Fatalf("got %d vpcs", len(vpcs))
	}
	if Value(vpcs["vpc-2"].CidrBlock) != "10.1.0.0/16" {
		t.Errorf("vpc-2 = %+v", vpcs["vpc-2"])
	}
}

func TestExists(t *testing.T) {
	swap(t, &fakeHost{listing: map[string]string{
		"cloudjam-demo": `{"BucketName":"cloudjam-demo"}`,
	}})

	got, err := Exists[*s3.Bucket]("cloudjam-demo")
	if err != nil || !got {
		t.Errorf("Exists(present) = %v, %v", got, err)
	}
	got, err = Exists[*s3.Bucket]("gone")
	if err != nil || got {
		t.Errorf("Exists(absent) = %v, %v", got, err)
	}
}

func TestRoundTrip(t *testing.T) {
	// What Create sends must be what Read can decode back.
	f := &fakeHost{}
	swap(t, f)

	original := &s3.Bucket{
		BucketName: String("cloudjam-demo"),
		PublicAccessBlockConfiguration: &s3.Bucket_PublicAccessBlockConfiguration{
			BlockPublicAcls:   Bool(true),
			IgnorePublicAcls:  Bool(false),
			BlockPublicPolicy: Bool(true),
		},
	}
	if err := Create(original); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Echo the desired state back with a read-only attribute bolted on, the way
	// Cloud Control would.
	var document map[string]json.RawMessage
	if err := json.Unmarshal([]byte(f.created.Desired), &document); err != nil {
		t.Fatalf("desired state is not an object: %v", err)
	}
	document["Arn"] = json.RawMessage(`"arn:aws:s3:::cloudjam-demo"`)
	echoed, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	f.state = string(echoed)

	got, err := Read[*s3.Bucket]("cloudjam-demo")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if Value(got.BucketName) != "cloudjam-demo" {
		t.Errorf("BucketName = %v", got.BucketName)
	}
	if !True(got.PublicAccessBlockConfiguration.BlockPublicAcls) ||
		True(got.PublicAccessBlockConfiguration.IgnorePublicAcls) {
		t.Errorf("public access block = %+v", got.PublicAccessBlockConfiguration)
	}
}

// TestPluginLifecycle drives the whole plugin surface — the same calls a real
// main would make — against the fake host.
func TestPluginLifecycle(t *testing.T) {
	f := &fakeHost{
		state:   `{"BucketName":"cloudjam-demo","BucketEncryption":{"ServerSideEncryptionConfiguration":[{"ServerSideEncryptionByDefault":{"SSEAlgorithm":"AES256"}}]}}`,
		listing: map[string]string{},
	}
	swap(t, f)

	c := New("Lock Down the Bucket", "Secure it.")
	c.Clue("encryption", "Look at BucketEncryption.")
	c.Interval(time.Millisecond)
	c.Timeout(10 * time.Millisecond)

	c.Add(&s3.Bucket{BucketName: String("cloudjam-demo")})

	c.Check("encrypted").
		Reason("Enabled default encryption").
		Points(50).
		Done(func() (bool, error) {
			bucket, err := Read[*s3.Bucket]("cloudjam-demo")
			if err != nil {
				return false, err
			}
			return bucket.BucketEncryption != nil, nil
		})

	c.Run()

	if f.meta.Title != "Lock Down the Bucket" || f.meta.Clues["encryption"] == "" {
		t.Errorf("meta = %+v", f.meta)
	}
	// Add always waits, and sends the bare properties.
	if f.created.Type != "AWS::S3::Bucket" || !f.created.Wait {
		t.Errorf("created = %+v", f.created)
	}
	if len(f.scores) != 1 || f.scores[0].Increment != 50 {
		t.Fatalf("scores = %+v, want exactly one award of 50", f.scores)
	}
	if f.scores[0].Reason != "Enabled default encryption" {
		t.Errorf("reason = %q", f.scores[0].Reason)
	}
	if len(f.reported) != 0 {
		t.Errorf("reported = %v, want nothing to have failed", f.reported)
	}
}

// TestChallengesAreIndependent: two challenges in one process must not share
// state, which is the point of New over a global.
func TestChallengesAreIndependent(t *testing.T) {
	f := &fakeHost{listing: map[string]string{}}
	swap(t, f)

	first := New("First", "")
	first.Timeout(time.Millisecond)
	first.Check("a").Points(1).Done(func() (bool, error) { return false, nil })

	second := New("Second", "")
	second.Check("b").Points(2).Done(func() (bool, error) { return true, nil })
	second.Run()

	if f.meta.Title != "Second" {
		t.Errorf("meta = %+v, want only the challenge that ran to register", f.meta)
	}
	if len(f.scores) != 1 || f.scores[0].Increment != 2 {
		t.Errorf("scores = %+v, want only the second challenge's check", f.scores)
	}
}
