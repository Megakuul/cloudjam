//go:build wasip1

// Command audit-day is an example cloudjam challenge plugin (standard tier).
//
// An external auditor has filed three findings against the bastion host. The
// player closes them, and then has to prove they did — the auditor does not
// take screenshots.
package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"codeberg.org/megakuul/cloudjam/pkg/challenge"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/policy"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/ec2"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/kms"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/logs"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/ssm"
	"github.com/google/uuid"
)

const (
	sshPort = 22
	rdpPort = 3389
)

// notePath is where the auditor wants the remediation note. The player is told
// this only when the auditor arrives.
const notePath = "/cloudjam/audit/remediation"

// retention the auditor will accept: long enough to investigate, short enough
// that nobody is paying to store last year's ssh logins.
const (
	minRetentionDays = 30
	maxRetentionDays = 400
)

// set by bootstrap before the check loop starts.
//
// groupRef doubles as the security group id, which is what the remediation note
// has to quote. It is captured here rather than re-read, because read-only
// attributes are not guaranteed to survive a player's update.
var (
	groupRef    string
	logGroupRef string
)

func main() {
	challenge.New("Audit Day", 10*time.Second, bootstrap).
		AddDescription(
			"The external auditor finished their sweep this morning and filed three findings " +
				"against the bastion. You have until their follow-up call to close them. " +
				"They were not impressed by the last team's answer of 'we'll get to it'.").
		AddDescription(
			"Finding 1: management ports on the bastion security group accept connections " +
				"from the entire internet.\n" +
				"Finding 2: the bastion log group has no retention policy — it either grows " +
				"forever or tells us nothing.\n" +
				"Finding 3: those logs are not encrypted with a key we control.").
		AddClue("finding 1", "The auditor's objection is the source range, not the port. Engineers still need to reach the bastion.").
		AddClue("finding 2", "Somewhere between a month and a year of logs is defensible. Forever is not.").
		AddClue("finding 3", "'A key we control' means a customer managed key, not the default service key.").
		SetPermission(policy.Document{
			Version: policy.Version20121017,
			Statement: []policy.Statement{
				{Effect: policy.Deny, Action: policy.ActionAll, Resource: policy.ARNAll}, // bootstrap grants the real thing.
			},
		}).
		SetGuardrail(policy.Document{
			Version: policy.Version20121017,
			Statement: []policy.Statement{
				// the player never needs iam, and the guardrail is what stops them
				// granting themselves more than the challenge intends.
				//
				// wildcards, not ActionsFrom: the boundary is a managed policy and IAM
				// caps those at 6144 characters. Expanding the four services' action
				// groups here would be 34991 and the boundary would never be created.
				{
					Effect:   policy.Allow,
					Action:   policy.Actions{"ec2:*", "logs:*", "kms:*", "ssm:*"},
					Resource: policy.ARNAll,
				},
			},
		}).
		AddCheck("Closed SSH to the internet without locking the team out", challenge.Check{
			Points:  50,
			Every:   15 * time.Second,
			Trigger: sshLockedDown,
		}).
		AddCheck("Took RDP off the internet", challenge.Check{
			Points:  30,
			Every:   15 * time.Second,
			Trigger: rdpClosed,
		}).
		AddCheck("Gave the bastion logs a retention window", challenge.Check{
			Points:  30,
			Every:   15 * time.Second,
			Trigger: retentionSet,
		}).
		AddCheck("Created a customer managed key", challenge.Check{
			Points:  25,
			Every:   15 * time.Second,
			Trigger: customerKeyExists,
		}).
		AddCheck("Encrypted the bastion logs with it", challenge.Check{
			Points:  35,
			Every:   15 * time.Second,
			Trigger: logsEncrypted,
		}).
		AddCheck("Kept the bastion closed", challenge.Check{
			Points:  5,
			Every:   time.Minute,
			Repeat:  true,
			Trigger: sshLockedDown,
		}).
		AddEvent("auditor", challenge.Event{
			Every:   20 * time.Second,
			Trigger: sshLockedDown, // the auditor turns up once the headline finding is closed.
			Event:   auditorArrives,
		}).
		Start()
}

func bootstrap(s *challenge.Scenario) error {
	group, err := aws.Create(&ec2.SecurityGroup{
		GroupName:        new(fmt.Sprintf("cloudjam-bastion-%s", uuid.NewString())),
		GroupDescription: new("bastion host - jump box for the platform team"),
		// the scenario: both management ports open to everybody.
		SecurityGroupIngress: []ec2.Ingress{
			{
				IpProtocol:  new("tcp"),
				FromPort:    new(sshPort),
				ToPort:      new(sshPort),
				CidrIp:      new("0.0.0.0/0"),
				Description: new("ssh"),
			},
			{
				IpProtocol:  new("tcp"),
				FromPort:    new(rdpPort),
				ToPort:      new(rdpPort),
				CidrIp:      new("0.0.0.0/0"),
				Description: new("rdp - nobody remembers who added this"),
			},
		},
	})
	if err != nil {
		return err
	}
	groupRef = group

	logGroup, err := aws.Create(&logs.LogGroup{
		LogGroupName: new(fmt.Sprintf("/cloudjam/bastion/%s", uuid.NewString())),
		// the scenario: no retention, no key.
	})
	if err != nil {
		return err
	}
	logGroupRef = logGroup

	// only the calls the three findings need. Naming them beats ActionsFrom on four
	// services: an inline policy is capped at 10240 characters and those action groups
	// expand to 34991 between them.
	s.SetPermission(policy.Document{
		Version: policy.Version20121017,
		Statement: []policy.Statement{
			{
				Sid:    "Remediation",
				Effect: policy.Allow,
				Action: policy.Actions{
					// finding 1: read the group, then take the internet off it.
					"ec2:Describe*",
					ec2.ActionAuthorizeSecurityGroupIngress,
					ec2.ActionRevokeSecurityGroupIngress,
					// finding 2: retention on the log group.
					"logs:Describe*",
					logs.ActionPutRetentionPolicy,
					// finding 3: a key of their own, and the log group encrypted with it.
					"kms:Describe*", "kms:List*", "kms:Get*",
					kms.ActionCreateKey, kms.ActionCreateAlias, kms.ActionPutKeyPolicy,
					kms.ActionEnableKeyRotation,
					logs.ActionAssociateKmsKey, logs.ActionDisassociateKmsKey,
					// act two: the remediation note.
					"ssm:DescribeParameters", "ssm:GetParameter*",
					ssm.ActionPutParameter, ssm.ActionAddTagsToResource,
				},
				// the key, and the note, do not exist yet, so they cannot be named here.
				Resource: policy.ARNAll,
			},
		},
	})
	return nil
}

// auditorArrives is the second act: the findings are closed, now prove it.
func auditorArrives(ctx context.Context, s *challenge.Scenario) error {
	s.AddDescription(fmt.Sprintf(
		"The auditor is back, and they want evidence rather than assurances. Two things "+
			"before they sign off: write a remediation note to the SSM parameter %q quoting "+
			"the id of the security group you fixed, and prove the key you created is on a "+
			"rotation schedule.", notePath))
	s.AddClue("evidence", fmt.Sprintf("The note is an SSM parameter at %q. Its value has to contain the security group id.", notePath))
	s.AddClue("rotation", "A customer managed key has a property that rotates its backing material on a schedule.")

	s.AddCheck("Filed the remediation note", challenge.Check{
		Points:  40,
		Every:   15 * time.Second,
		Trigger: noteFiled,
	})
	s.AddCheck("Put the audit key on a rotation schedule", challenge.Check{
		Points:  30,
		Every:   15 * time.Second,
		Trigger: keyRotationEnabled,
	})
	return nil
}

// sshLockedDown wants the port reachable from somewhere and not from
// everywhere. Deleting the rule outright is not a fix — the team still has to
// get in — so it does not pass.
func sshLockedDown() (bool, error) {
	group, err := readGroup()
	if err != nil {
		return false, err
	}
	reachable := false
	for _, rule := range group.SecurityGroupIngress {
		if !covers(rule, sshPort) {
			continue
		}
		if openToWorld(rule) {
			return false, nil
		}
		reachable = true
	}
	return reachable, nil
}

// rdpClosed accepts either answer: delete the rule, or restrict it.
func rdpClosed() (bool, error) {
	group, err := readGroup()
	if err != nil {
		return false, err
	}
	for _, rule := range group.SecurityGroupIngress {
		if covers(rule, rdpPort) && openToWorld(rule) {
			return false, nil
		}
	}
	return true, nil
}

// covers reports whether an ingress rule lets traffic reach a port. A protocol
// of "-1" is every protocol and every port, however the port range reads.
func covers(rule ec2.Ingress, port int) bool {
	if rule.IpProtocol != nil && *rule.IpProtocol == "-1" {
		return true
	}
	if rule.FromPort == nil || rule.ToPort == nil {
		return false
	}
	return *rule.FromPort <= port && port <= *rule.ToPort
}

func openToWorld(rule ec2.Ingress) bool {
	if rule.CidrIp != nil && *rule.CidrIp == "0.0.0.0/0" {
		return true
	}
	return rule.CidrIpv6 != nil && *rule.CidrIpv6 == "::/0"
}

func retentionSet() (bool, error) {
	group, err := readLogGroup()
	if err != nil {
		return false, err
	}
	if group.RetentionInDays == nil {
		return false, nil
	}
	return *group.RetentionInDays >= minRetentionDays && *group.RetentionInDays <= maxRetentionDays, nil
}

func customerKeyExists() (bool, error) {
	keys, err := aws.List[*kms.Key]()
	if err != nil {
		return false, err
	}
	return len(keys) > 0, nil
}

func logsEncrypted() (bool, error) {
	group, err := readLogGroup()
	if err != nil {
		return false, err
	}
	return group.KmsKeyId != nil && *group.KmsKeyId != "", nil
}

func keyRotationEnabled() (bool, error) {
	keys, err := aws.List[*kms.Key]()
	if err != nil {
		return false, err
	}
	for _, key := range keys {
		if key.EnableKeyRotation != nil && *key.EnableKeyRotation {
			return true, nil
		}
	}
	return false, nil
}

// noteFiled wants the note to quote the security group id, so the player has to
// have actually looked at the thing they fixed.
func noteFiled() (bool, error) {
	if groupRef == "" {
		return false, fmt.Errorf("security group was never provisioned")
	}
	// List rather than Read: until the player writes the note it does not exist,
	// and reading a missing resource logs a 404 on the host every single cycle.
	// List tells absent from unreadable without the noise.
	notes, err := aws.List[*ssm.Parameter]()
	if err != nil {
		return false, err
	}
	note, ok := notes[notePath]
	if !ok || note.Value == nil {
		return false, nil
	}
	return strings.Contains(*note.Value, groupRef), nil
}

func readGroup() (*ec2.SecurityGroup, error) {
	if groupRef == "" {
		return nil, fmt.Errorf("security group was never provisioned")
	}
	return aws.Read[*ec2.SecurityGroup](groupRef)
}

func readLogGroup() (*logs.LogGroup, error) {
	if logGroupRef == "" {
		return nil, fmt.Errorf("log group was never provisioned")
	}
	return aws.Read[*logs.LogGroup](logGroupRef)
}
