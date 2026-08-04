package aws

import (
	"fmt"

	"codeberg.org/megakuul/cloudjam/pkg/challenge"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/api"
)

// Challenge is an AWS challenge. It is a challenge.Challenge that also takes
// typed AWS resources, so Clue, Interval, Timeout, Check and Run come straight
// from the embedded type.
type Challenge struct {
	*challenge.Challenge
}

// New builds an AWS challenge with the title and briefing shown to the player:
//
//	c := aws.New("Lock Down the Bucket", "A bucket has no encryption. Fix it.")
//	c.Add(&s3.Bucket{BucketName: aws.String("cloudjam-demo")})
//	c.Check("encrypted").Points(50).Done(encrypted)
//	c.Run()
func New(title, description string) *Challenge {
	return &Challenge{challenge.New(hostAdapter{}, title, description)}
}

// Add declares a resource the scenario needs. It is created when Run starts and
// retried every round until that succeeds, with failures written to the host's
// report endpoint — so there is no error to handle here:
//
//	c.Add(&s3.Bucket{BucketName: aws.String("cloudjam-demo")})
//
// It only ensures the resource exists. It deliberately does not enforce its
// properties: the player is supposed to change them, and an enforcing loop would
// undo their work.
func (c *Challenge) Add(resource Resource) {
	typeName := resource.AWSCloudFormationType()
	c.Provision(typeName, func() error {
		desired, err := marshalResource(resource)
		if err != nil {
			return err
		}
		// Wait, so a check in the same round sees a resource that is really there.
		return createResource(typeName, desired, true)
	})
}

// Score returns the player's current score.
func Score() (float64, error) {
	out, err := host.ReadScore(api.ReadScoreInput{})
	if err != nil {
		return 0, fmt.Errorf("read score: %w", err)
	}
	return out.Score, nil
}

// Log writes a diagnostic message to the host's log.
func Log(msg string) { host.Log(msg) }

// Logf writes a formatted diagnostic message to the host's log.
func Logf(format string, args ...any) { host.Log(fmt.Sprintf(format, args...)) }

// hostAdapter presents this package's transport as a challenge.Host.
type hostAdapter struct{}

func (hostAdapter) Register(meta api.InitInput) error {
	_, err := host.Init(meta)
	return err
}

func (hostAdapter) Report(message string) error {
	_, err := host.Report(api.ReportInput{Error: message})
	return err
}

func (hostAdapter) Award(reason string, points float64) error {
	_, err := host.UpdateScore(api.UpdateScoreInput{Reason: reason, Increment: points})
	return err
}

func (hostAdapter) Log(message string) { host.Log(message) }
