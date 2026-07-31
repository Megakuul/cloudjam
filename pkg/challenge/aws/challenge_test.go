package aws

import (
	"testing"

	"codeberg.org/megakuul/cloudjam/pkg/challenge"
	"github.com/awslabs/goformation/v7/cloudformation/s3"
)

// fakeHost is an in-memory host used to exercise the pure lifecycle without WASM.
type fakeHost struct {
	score      float64
	meta       challenge.InitInput
	created    map[string]string // typeName -> desired
	state      map[string]string // typeName -> current properties JSON
	completed  map[string]bool
	scoreCalls int
}

func newFakeHost() *fakeHost {
	return &fakeHost{
		created:   map[string]string{},
		state:     map[string]string{},
		completed: map[string]bool{},
	}
}

func (f *fakeHost) RegisterMeta(m challenge.InitInput) error { f.meta = m; return nil }
func (f *fakeHost) ReadScore() (float64, error)              { return f.score, nil }
func (f *fakeHost) UpdateScore(_ string, inc float64) error {
	f.score += inc
	f.scoreCalls++
	return nil
}

func (f *fakeHost) CreateResource(t, desired string) error {
	f.created[t] = desired
	f.state[t] = desired // a real host returns the created resource on subsequent reads
	return nil
}
func (f *fakeHost) ReadResource(t, _ string) (string, error) { return f.state[t], nil }
func (f *fakeHost) UpdateResource(_, _, _ string) error      { return nil }
func (f *fakeHost) DeleteResource(_, _ string) error         { return nil }
func (f *fakeHost) Completed(id string) (bool, error)        { return f.completed[id], nil }
func (f *fakeHost) MarkCompleted(id string) error            { f.completed[id] = true; return nil }
func (f *fakeHost) Log(string)                               {}

const testBucket = "unit-bucket"

func testChallenge() *Challenge {
	return New(Config{
		Title:       "unit",
		Description: "d",
		Clues:       []Clue{{Text: "hint", Cost: 3}},
		Setup: func(c *Context) error {
			return c.Create(&s3.Bucket{BucketName: new(testBucket)})
		},
		Objectives: []Objective{
			{
				ID:     "encrypted",
				Points: 50,
				Check: func(c *Context) (bool, error) {
					var b s3.Bucket
					if err := c.Read(testBucket, &b); err != nil {
						return false, err
					}
					return b.BucketEncryption != nil, nil
				},
			},
		},
	})
}

func TestInitProvisionsAndRegistersMeta(t *testing.T) {
	h := newFakeHost()
	if err := testChallenge().run(h, phaseInit); err != nil {
		t.Fatalf("init: %v", err)
	}
	if h.meta.Title != "unit" {
		t.Errorf("title = %q, want unit", h.meta.Title)
	}
	if h.meta.Clues["hint"] != 3 {
		t.Errorf("clue cost = %v, want 3", h.meta.Clues["hint"])
	}
	desired, ok := h.created["AWS::S3::Bucket"]
	if !ok {
		t.Fatal("bucket was not provisioned")
	}
	if want := `{"BucketName":"unit-bucket"}`; desired != want {
		t.Errorf("desired = %s, want %s", desired, want)
	}
	if h.score != 0 {
		t.Errorf("score = %v, want 0 (objective not yet met)", h.score)
	}
}

func TestObjectiveAwardsOnceWhenStateReached(t *testing.T) {
	h := newFakeHost()
	c := testChallenge()
	if err := c.run(h, phaseInit); err != nil {
		t.Fatalf("init: %v", err)
	}

	// Player enables encryption; note the read-only Arn attribute must be tolerated.
	h.state["AWS::S3::Bucket"] = `{"BucketName":"unit-bucket","Arn":"arn:aws:s3:::unit-bucket",` +
		`"BucketEncryption":{"ServerSideEncryptionConfiguration":[{"ServerSideEncryptionByDefault":{"SSEAlgorithm":"AES256"}}]}}`

	if err := c.run(h, phaseEvaluate); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if h.score != 50 {
		t.Fatalf("score = %v, want 50", h.score)
	}

	// Re-evaluate: must not pay twice.
	if err := c.run(h, phaseEvaluate); err != nil {
		t.Fatalf("re-evaluate: %v", err)
	}
	if h.score != 50 {
		t.Errorf("score after re-evaluate = %v, want 50", h.score)
	}
	if h.scoreCalls != 1 {
		t.Errorf("UpdateScore calls = %d, want 1", h.scoreCalls)
	}
}

func TestValidateRejectsBadConfig(t *testing.T) {
	cases := map[string]Config{
		"no title":  {Objectives: nil},
		"empty id":  {Title: "t", Objectives: []Objective{{Check: func(*Context) (bool, error) { return false, nil }}}},
		"nil check": {Title: "t", Objectives: []Objective{{ID: "x"}}},
		"duplicate": {Title: "t", Objectives: []Objective{
			{ID: "x", Check: func(*Context) (bool, error) { return false, nil }},
			{ID: "x", Check: func(*Context) (bool, error) { return false, nil }},
		}},
	}
	for name, cfg := range cases {
		if err := New(cfg).validate(); err == nil {
			t.Errorf("%s: expected validation error, got nil", name)
		}
	}
}
