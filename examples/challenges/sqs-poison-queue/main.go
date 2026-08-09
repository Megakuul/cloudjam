//go:build wasip1

// Command sqs-poison-queue is an example cloudjam challenge plugin (warmup tier).
//
// One order in the checkout queue cannot be processed. With nowhere to put a
// failure the queue redelivers it forever, and every order behind it waits.
// The player gives the queue a dead-letter queue and room to breathe.
package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"codeberg.org/megakuul/cloudjam/pkg/challenge"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/policy"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/sqs"
	"github.com/google/uuid"
)

const queuePrefix = "cloudjam-checkout"

// what the scenario ships with: ten seconds is less than the consumer needs, so
// every order goes back on the queue before it can finish.
const (
	brokenVisibilityTimeout = 10
	brokenRetentionPeriod   = 3600 // one hour, so evidence disappears too
)

// what the player has to reach.
const (
	wantVisibilityTimeout = 60
	wantRetentionPeriod   = 4 * 24 * 60 * 60
)

// queueRef is the primary identifier of the checkout queue, set by bootstrap
// before the check loop starts.
var queueRef string

func main() {
	challenge.New("The Order That Would Not Die", 10*time.Second, bootstrap).
		AddDescription(
			"Checkout has been stuck for twenty minutes. The payments consumer picks up the " +
				"same order, times out, and the order goes straight back on the queue — and " +
				"three thousand orders behind it are waiting their turn. Give the failure " +
				"somewhere to go.").
		AddClue("stuck", "A message that cannot be processed needs somewhere to go after N attempts. SQS calls that a redrive policy.").
		AddClue("timeout", "The consumer needs about 45 seconds. How long does SQS hide a message it has handed out?").
		AddClue("evidence", "Once the queue drains, you still want the bad order to look at. How long does SQS keep a message?").
		SetPermission(policy.Document{
			Version: policy.Version20121017,
			Statement: []policy.Statement{
				{Effect: policy.Deny, Action: policy.ActionAll, Resource: policy.ARNAll}, // bootstrap grants the real thing.
			},
		}).
		SetGuardrail(policy.Document{
			Version: policy.Version20121017,
			Statement: []policy.Statement{
				// the challenge touches no iam, so the guardrail only has to keep
				// the player inside sqs.
				{Effect: policy.Allow, Action: policy.ActionsFrom(sqs.ActionsRead, sqs.ActionsWrite), Resource: policy.ARNAll},
			},
		}).
		AddCheck("Stood up a dead-letter queue", challenge.Check{
			Points:  25,
			Every:   15 * time.Second,
			Trigger: deadLetterQueueExists,
		}).
		AddCheck("Pointed the checkout queue at it", challenge.Check{
			Points:  40,
			Every:   15 * time.Second,
			Trigger: redriveConfigured,
		}).
		AddCheck("Gave the consumer time to finish", challenge.Check{
			Points:  20,
			Every:   15 * time.Second,
			Trigger: visibilityRaised,
		}).
		AddCheck("Kept the failed order long enough to read it", challenge.Check{
			Points:  15,
			Every:   15 * time.Second,
			Trigger: retentionRaised,
		}).
		Start()
}

func bootstrap(s *challenge.Scenario) error {
	ref, err := aws.Create(&sqs.Queue{
		QueueName:              new(fmt.Sprintf("%s-%s", queuePrefix, uuid.NewString())),
		VisibilityTimeout:      new(brokenVisibilityTimeout),
		MessageRetentionPeriod: new(brokenRetentionPeriod),
	})
	if err != nil {
		return err
	}
	queueRef = ref

	// the player needs to read the broken queue, patch it, and create the
	// dead-letter queue next to it.
	s.SetPermission(policy.Document{
		Version: policy.Version20121017,
		Statement: []policy.Statement{
			{
				Sid:      "QueueAccess",
				Effect:   policy.Allow,
				Action:   policy.ActionsFrom(sqs.ActionsRead, sqs.ActionsWrite),
				Resource: policy.ARNAll, // the dead-letter queue does not exist yet, so it cannot be named here.
			},
		},
	})
	return nil
}

// deadLetterQueueExists looks for a queue that is not the checkout queue. It
// deliberately does not care what the player called it.
func deadLetterQueueExists() (bool, error) {
	if queueRef == "" {
		return false, fmt.Errorf("checkout queue was never provisioned")
	}
	queues, err := aws.List[*sqs.Queue]()
	if err != nil {
		return false, err
	}
	for identifier := range queues {
		if identifier != queueRef {
			return true, nil
		}
	}
	return false, nil
}

func redriveConfigured() (bool, error) {
	q, err := readQueue()
	if err != nil {
		return false, err
	}
	if len(q.RedrivePolicy) == 0 {
		return false, nil
	}
	// RedrivePolicy is free-form json on the resource, and aws is inconsistent
	// about whether maxReceiveCount comes back as a number or a string.
	var raw map[string]any
	if err := json.Unmarshal(q.RedrivePolicy, &raw); err != nil {
		return false, nil // the player wrote something unreadable; that is a wrong answer, not our error.
	}
	target, _ := raw["deadLetterTargetArn"].(string)
	if target == "" {
		return false, nil
	}
	return maxReceiveCount(raw["maxReceiveCount"]) > 0, nil
}

func maxReceiveCount(value any) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case string:
		count, _ := strconv.Atoi(v)
		return count
	}
	return 0
}

func visibilityRaised() (bool, error) {
	q, err := readQueue()
	if err != nil {
		return false, err
	}
	if q.VisibilityTimeout == nil {
		return false, nil
	}
	return *q.VisibilityTimeout >= wantVisibilityTimeout, nil
}

func retentionRaised() (bool, error) {
	q, err := readQueue()
	if err != nil {
		return false, err
	}
	if q.MessageRetentionPeriod == nil {
		return false, nil
	}
	return *q.MessageRetentionPeriod >= wantRetentionPeriod, nil
}

// readQueue reads the checkout queue. It reports rather than awards when the
// queue is missing, so a failed bootstrap cannot hand out points.
func readQueue() (*sqs.Queue, error) {
	if queueRef == "" {
		return nil, fmt.Errorf("checkout queue was never provisioned")
	}
	return aws.Read[*sqs.Queue](queueRef)
}
