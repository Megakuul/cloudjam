//go:build wasip1

// Command dynamodb-no-backups is an example cloudjam challenge plugin (warmup tier).
//
// The sessions table survived last night's incident by luck. The player makes
// sure the next one is survivable on purpose.
package main

import (
	"fmt"
	"time"

	"codeberg.org/megakuul/cloudjam/pkg/challenge"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/policy"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/dynamodb"
	"github.com/google/uuid"
)

const tablePrefix = "cloudjam-sessions"

// ownerTag is the tag the platform team wants on anything that holds customer
// data. bootstrap does not set it — the player has to.
const ownerTag = "data-owner"

// tableRef is the primary identifier of the sessions table, set by bootstrap
// before the check loop starts.
var tableRef string

func main() {
	challenge.New("Nobody Owns the Sessions Table", 10*time.Second, bootstrap).
		AddDescription(
			"At 02:14 someone ran a cleanup script against the wrong table and deleted half "+
				"of it. We got lucky: the sessions rebuilt themselves from cache. The "+
				"post-incident action is on you — make the sessions table recoverable, and "+
				"make it obvious who to call next time.").
		// clue prices are added to the team score, so they are negative.
		AddClue("recovery", "DynamoDB can restore a table to any second in the recent past, if you ask it to in advance.", -10).
		AddClue("accident", "The cleanup script was allowed to call DeleteTable. There is a table-level setting for that.", -7).
		AddClue("ownership", fmt.Sprintf("The platform team wants a %q tag on anything holding customer data.", ownerTag), -5).
		SetPermission(policy.Document{
			Version: policy.Version20121017,
			Statement: []policy.Statement{
				{Effect: policy.Deny, Action: policy.ActionAll, Resource: policy.ARNAll}, // bootstrap grants the real thing.
			},
		}).
		SetGuardrail(policy.Document{
			Version: policy.Version20121017,
			Statement: []policy.Statement{
				{
					Effect:   policy.Allow,
					Action:   policy.ActionsFrom(dynamodb.ActionsRead, dynamodb.ActionsList, dynamodb.ActionsWrite, dynamodb.ActionsTagging),
					Resource: policy.ARNAll,
				},
			},
		}).
		AddCheck("Turned on point-in-time recovery", challenge.Check{
			Points:  45,
			Every:   15 * time.Second,
			Trigger: recoveryEnabled,
		}).
		AddCheck("Stopped the table being deletable by accident", challenge.Check{
			Points:  30,
			Every:   15 * time.Second,
			Trigger: deletionProtected,
		}).
		AddCheck("Named an owner for the table", challenge.Check{
			Points:  25,
			Every:   15 * time.Second,
			Trigger: ownerNamed,
		}).
		AddCheck("Kept the table recoverable", challenge.Check{
			Points:  5,
			Every:   time.Minute,
			Repeat:  true,
			Trigger: recoveryEnabled,
		}).
		Start()
}

func bootstrap(s *challenge.Scenario) error {
	name := fmt.Sprintf("%s-%s", tablePrefix, uuid.NewString())
	ref, err := aws.Create(&dynamodb.Table{
		TableName:   new(name),
		BillingMode: new("PAY_PER_REQUEST"), // on demand: no capacity to leave running.
		AttributeDefinitions: []dynamodb.TableAttributeDefinition{
			{AttributeName: new("sessionId"), AttributeType: new("S")},
		},
		KeySchema: []byte(`[{"AttributeName":"sessionId","KeyType":"HASH"}]`),
		// the scenario: no recovery, no deletion protection, no owner.
		DeletionProtectionEnabled: new(false),
	})
	if err != nil {
		return err
	}
	tableRef = ref

	s.SetPermission(policy.Document{
		Version: policy.Version20121017,
		Statement: []policy.Statement{
			{
				Sid:      "TableAccess",
				Effect:   policy.Allow,
				Action:   policy.ActionsFrom(dynamodb.ActionsRead, dynamodb.ActionsList, dynamodb.ActionsWrite, dynamodb.ActionsTagging),
				Resource: policy.ARNsFrom(fmt.Sprintf("arn:aws:dynamodb:*:*:table/%s", name)),
			},
		},
	})
	return nil
}

func recoveryEnabled() (bool, error) {
	t, err := readTable()
	if err != nil {
		return false, err
	}
	spec := t.PointInTimeRecoverySpecification
	if spec == nil || spec.PointInTimeRecoveryEnabled == nil {
		return false, nil
	}
	return *spec.PointInTimeRecoveryEnabled, nil
}

func deletionProtected() (bool, error) {
	t, err := readTable()
	if err != nil {
		return false, err
	}
	if t.DeletionProtectionEnabled == nil {
		return false, nil
	}
	return *t.DeletionProtectionEnabled, nil
}

// ownerNamed accepts any non-empty owner. Checking for a particular name would
// be checking that the player guessed, not that they did the work.
func ownerNamed() (bool, error) {
	t, err := readTable()
	if err != nil {
		return false, err
	}
	for _, tag := range t.Tags {
		if tag.Key == nil || *tag.Key != ownerTag {
			continue
		}
		if tag.Value != nil && *tag.Value != "" {
			return true, nil
		}
	}
	return false, nil
}

// readTable reads the sessions table. It reports rather than awards when the
// table is missing, so a failed bootstrap cannot hand out points.
func readTable() (*dynamodb.Table, error) {
	if tableRef == "" {
		return nil, fmt.Errorf("sessions table was never provisioned")
	}
	return aws.Read[*dynamodb.Table](tableRef)
}
