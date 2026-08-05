package aws

import (
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
//
// The returned Ref carries the primary identifier once the resource exists,
// which is how a check reaches something AWS named itself. Ignore it for
// resources you named yourself:
//
//	vpc := c.Add(&ec2.VPC{CidrBlock: aws.String("10.0.0.0/16")})
//	c.Check("subnet-added").Done(func() (bool, error) {
//		return subnetIn(vpc.ID())
//	})
func (c *Challenge) Add(resource Resource) {
	typeName := resource.AWSCloudFormationType()
	c.Provision(typeName, func() error {
		desired, err := marshalResource(resource)
		if err != nil {
			return err
		}
		status, err := api.CreateResource(api.CreateResourceInput{Type: typeName, Desired: string(desired)})
		if err != nil {
			return err
		}
		ref.identifier = status.Identifier
		return nil
	})
	return ref
}
