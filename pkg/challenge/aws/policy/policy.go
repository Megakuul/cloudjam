// Package policy provides useful constructors and parsers for AWS policy documents.
// This avoids manual parsing / handling in every challenge plugin. Should be extended with useful helpers that avoid plugin boilerplate for policies.
package policy

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

type Document struct {
	Version   Version     `json:"Version"`
	ID        string      `json:"Id,omitempty"`
	Statement []Statement `json:"Statement"`
}

func (d Document) String() string {
	raw, _ := json.Marshal(d)
	return string(raw)
}

type Statement struct {
	Sid          string     `json:"Sid,omitempty"`
	Effect       Effect     `json:"Effect"`
	Principal    *Principal `json:"Principal,omitempty"`
	NotPrincipal *Principal `json:"NotPrincipal,omitempty"`
	Action       Actions    `json:"Action,omitempty"`
	NotAction    Actions    `json:"NotAction,omitempty"`
	Resource     ARNs       `json:"Resource,omitempty"`
	NotResource  ARNs       `json:"NotResource,omitempty"`
	Condition    Conditions `json:"Condition,omitempty"`
}

type Version string

const Version20121017 Version = "2012-10-17"

type Effect string

const (
	Allow Effect = "Allow"
	Deny  Effect = "Deny"
)

// Action is an IAM action such as "s3:GetObject". Every action of every
// service is generated as a constant in that service's package, so prefer
// s3.ActionGetObject over the literal.
type Action string

var ActionAll Actions = Actions{"*"}

// ARN is a resource an action applies to.
type ARN string

var ARNAll ARNs = ARNs{"*"}

type Actions []Action

type ARNs []ARN

// ActionsFrom generates an actions list from specified actions or action groups.
//
//	Action: aws.ActionsFrom(s3.ActionsRead, s3.ActionsList)
func ActionsFrom(lists ...[]string) Actions {
	out := Actions{}
	for _, list := range lists {
		for _, name := range list {
			out = append(out, Action(name))
		}
	}
	return out
}

// ARNsFrom generates a resource list from specified ARNs.
//
//	ARNs: aws.ARNsFrom("arn:aws:s3:::yourbucket/*")
func ARNsFrom(arns ...string) ARNs {
	out := ARNs{}
	for _, arn := range arns {
		out = append(out, ARN(arn))
	}
	return out
}

// ParsePolicy reads a policy back, so a check can assert on what the player
// actually wrote.
func ParsePolicy(raw []byte) (Document, error) {
	var doc Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return doc, fmt.Errorf("parse policy: %w", err)
	}
	return doc, nil
}

func (d Document) MarshalJSON() ([]byte, error) {
	type document Document // shed the method so this does not recurse
	if d.Version == "" {
		d.Version = Version20121017
	}
	return json.Marshal(document(d))
}

// Principal is who a statement applies to. Everyone wins over the rest and
// renders as the bare "*".
type Principal struct {
	Everyone      bool
	AWS           []string
	Service       []string
	Federated     []string
	CanonicalUser []string
}

func PrincipalEveryone() *Principal               { return &Principal{Everyone: true} }
func PrincipalAWS(arns ...string) *Principal      { return &Principal{AWS: arns} }
func PrincipalService(ids ...string) *Principal   { return &Principal{Service: ids} }
func PrincipalFederated(ids ...string) *Principal { return &Principal{Federated: ids} }

func (p Principal) MarshalJSON() ([]byte, error) {
	if p.Everyone {
		return json.Marshal("*")
	}
	out := map[string]any{}
	for key, values := range map[string][]string{
		"AWS":           p.AWS,
		"Service":       p.Service,
		"Federated":     p.Federated,
		"CanonicalUser": p.CanonicalUser,
	} {
		if len(values) > 0 {
			out[key] = collapse(values)
		}
	}
	return json.Marshal(out)
}

func (p *Principal) UnmarshalJSON(raw []byte) error {
	var everyone string
	if json.Unmarshal(raw, &everyone) == nil {
		p.Everyone = everyone == "*"
		return nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return err
	}
	for key, value := range object {
		values, err := expand(value)
		if err != nil {
			return err
		}
		switch key {
		case "AWS":
			p.AWS = values
		case "Service":
			p.Service = values
		case "Federated":
			p.Federated = values
		case "CanonicalUser":
			p.CanonicalUser = values
		}
	}
	return nil
}

// ConditionOperator is an IAM condition operator. Use the constructors below;
// this type is exported so operators they do not cover stay reachable.
type ConditionOperator string

// ConditionKey is what a condition tests. The global aws: keys are named
// below; a service's own keys are generated into its package as s3.KeyXAmzAcl
// and friends, with the ones that take a value generated as functions.
type ConditionKey string

const (
	KeySourceIP               ConditionKey = "aws:SourceIp"
	KeySourceArn              ConditionKey = "aws:SourceArn"
	KeySourceAccount          ConditionKey = "aws:SourceAccount"
	KeySecureTransport        ConditionKey = "aws:SecureTransport"
	KeyPrincipalArn           ConditionKey = "aws:PrincipalArn"
	KeyPrincipalAccount       ConditionKey = "aws:PrincipalAccount"
	KeyPrincipalOrgID         ConditionKey = "aws:PrincipalOrgID"
	KeyRequestedRegion        ConditionKey = "aws:RequestedRegion"
	KeyMultiFactorAuthPresent ConditionKey = "aws:MultiFactorAuthPresent"
	KeyCurrentTime            ConditionKey = "aws:CurrentTime"
	KeyUserAgent              ConditionKey = "aws:UserAgent"
	KeyViaAWSService          ConditionKey = "aws:ViaAWSService"
	KeyUsername               ConditionKey = "aws:username"
	KeyUserID                 ConditionKey = "aws:userid"
)

func Key(raw string) ConditionKey             { return ConditionKey(raw) }
func KeyRequestTag(tag string) ConditionKey   { return ConditionKey("aws:RequestTag/" + tag) }
func KeyPrincipalTag(tag string) ConditionKey { return ConditionKey("aws:PrincipalTag/" + tag) }
func KeyResourceTag(tag string) ConditionKey  { return ConditionKey("aws:ResourceTag/" + tag) }

// Condition is one operator applied to one key. Conditions on the same
// operator and key are merged when the statement is written.
type Condition struct {
	Operator ConditionOperator
	Key      ConditionKey
	Values   []string
}

// IfExists lets the statement match when the key is absent.
func (c Condition) IfExists() Condition {
	if !strings.HasSuffix(string(c.Operator), "IfExists") {
		c.Operator += "IfExists"
	}
	return c
}

// ForAnyValue matches when any value of a multi-valued key matches.
func (c Condition) ForAnyValue() Condition { return c.qualify("ForAnyValue:") }

// ForAllValues matches only when every value of a multi-valued key matches.
func (c Condition) ForAllValues() Condition { return c.qualify("ForAllValues:") }

func (c Condition) qualify(prefix string) Condition {
	c.Operator = ConditionOperator(prefix) + c.Operator
	return c
}

func StringEquals(key ConditionKey, values ...string) Condition {
	return Condition{"StringEquals", key, values}
}

func StringNotEquals(key ConditionKey, values ...string) Condition {
	return Condition{"StringNotEquals", key, values}
}

func StringLike(key ConditionKey, values ...string) Condition {
	return Condition{"StringLike", key, values}
}

func StringNotLike(key ConditionKey, values ...string) Condition {
	return Condition{"StringNotLike", key, values}
}

func ArnEquals(key ConditionKey, values ...string) Condition {
	return Condition{"ArnEquals", key, values}
}

func ArnLike(key ConditionKey, values ...string) Condition {
	return Condition{"ArnLike", key, values}
}

func IPAddress(key ConditionKey, cidrs ...string) Condition {
	return Condition{"IpAddress", key, cidrs}
}

func NotIPAddress(key ConditionKey, cidrs ...string) Condition {
	return Condition{"NotIpAddress", key, cidrs}
}

// BoolIs writes the value as the string IAM expects, which is the usual way a
// hand written Bool condition goes wrong.
func BoolIs(key ConditionKey, value bool) Condition {
	return Condition{"Bool", key, []string{strconv.FormatBool(value)}}
}

// Null tests whether a key is absent (true) or present (false).
func Null(key ConditionKey, absent bool) Condition {
	return Condition{"Null", key, []string{strconv.FormatBool(absent)}}
}

func NumericLessThan(key ConditionKey, value int) Condition {
	return Condition{"NumericLessThan", key, []string{strconv.Itoa(value)}}
}

func NumericGreaterThan(key ConditionKey, value int) Condition {
	return Condition{"NumericGreaterThan", key, []string{strconv.Itoa(value)}}
}

func DateLessThan(key ConditionKey, value time.Time) Condition {
	return Condition{"DateLessThan", key, []string{value.UTC().Format(time.RFC3339)}}
}

func DateGreaterThan(key ConditionKey, value time.Time) Condition {
	return Condition{"DateGreaterThan", key, []string{value.UTC().Format(time.RFC3339)}}
}

// Conditions is the statement's condition block. It is a list rather than the
// nested map IAM uses, so a policy reads top to bottom.
type Conditions []Condition

func (c Conditions) MarshalJSON() ([]byte, error) {
	byOperator := map[ConditionOperator]map[ConditionKey][]string{}
	for _, condition := range c {
		if byOperator[condition.Operator] == nil {
			byOperator[condition.Operator] = map[ConditionKey][]string{}
		}
		byOperator[condition.Operator][condition.Key] = append(
			byOperator[condition.Operator][condition.Key], condition.Values...)
	}

	out := map[ConditionOperator]map[ConditionKey]any{}
	for operator, keys := range byOperator {
		out[operator] = map[ConditionKey]any{}
		for key, values := range keys {
			out[operator][key] = collapse(values)
		}
	}
	return json.Marshal(out)
}

func (c *Conditions) UnmarshalJSON(raw []byte) error {
	var nested map[ConditionOperator]map[ConditionKey]json.RawMessage
	if err := json.Unmarshal(raw, &nested); err != nil {
		return err
	}
	for _, operator := range sortedKeys(nested) {
		for _, key := range sortedKeys(nested[operator]) {
			values, err := expand(nested[operator][key])
			if err != nil {
				return err
			}
			*c = append(*c, Condition{Operator: operator, Key: key, Values: values})
		}
	}
	return nil
}

func (a Actions) MarshalJSON() ([]byte, error) { return marshalList(a) }

func (a *Actions) UnmarshalJSON(raw []byte) error { return unmarshalList(raw, a) }

func (a ARNs) MarshalJSON() ([]byte, error) { return marshalList(a) }

func (a *ARNs) UnmarshalJSON(raw []byte) error { return unmarshalList(raw, a) }

// marshalList writes a single element as a bare string, which is how AWS
// renders policies and how anyone reading one expects it to look.
func marshalList[T ~string](list []T) ([]byte, error) {
	if len(list) == 1 {
		return json.Marshal(string(list[0]))
	}
	plain := make([]string, len(list))
	for i, value := range list {
		plain[i] = string(value)
	}
	return json.Marshal(plain)
}

func unmarshalList[S ~[]T, T ~string](raw []byte, out *S) error {
	values, err := expand(raw)
	if err != nil {
		return err
	}
	*out = make(S, len(values))
	for i, value := range values {
		(*out)[i] = T(value)
	}
	return nil
}

// collapse renders a single value as a string and anything else as a list.
func collapse(values []string) any {
	if len(values) == 1 {
		return values[0]
	}
	return values
}

// expand reads a value that is either a string or a list of them.
func expand(raw json.RawMessage) ([]string, error) {
	var single string
	if json.Unmarshal(raw, &single) == nil {
		return []string{single}, nil
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("expected a string or a list of strings, got %s", raw)
	}
	return list, nil
}

func sortedKeys[K ~string, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
