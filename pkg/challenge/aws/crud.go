package aws

import (
	"encoding/json"
	"fmt"

	"codeberg.org/megakuul/cloudjam/pkg/challenge/api"
)

// Create provisions a resource from its typed goformation definition and returns
// its primary identifier. Both type parameters are inferred from the argument:
//
//	id, err := aws.Create(&ec2.VPC{CidrBlock: aws.String("10.0.0.0/16")})
//	// id == "vpc-0a1b2c3d"
//
// The identifier is the only way to reach a resource AWS names itself, so keep
// it. For resources you name (a bucket, a role) it is the name you set.
//
// Every mutating call blocks until the Cloud Control operation reaches a
// terminal state, so once this returns the resource is really there.
func Create[P ResourceOf[T], T any](desired P) (string, error) {
	properties, err := marshalResource(desired)
	if err != nil {
		return "", err
	}
	return createResource(TypeName[P, T](), properties)
}

// Read loads a resource's live state into its typed goformation struct. The type
// parameter selects the resource type, the argument its primary identifier:
//
//	bucket, err := aws.Read[*s3.Bucket]("cloudjam-demo")
//	encrypted := bucket.BucketEncryption != nil
//
// Read-only attributes the struct does not model are ignored. Use ReadState when
// you need them.
func Read[P ResourceOf[T], T any](identifier string) (P, error) {
	state, err := readResource(TypeName[P, T](), identifier)
	if err != nil {
		return nil, err
	}
	return unmarshalState[P, T](state)
}

// ReadState is Read plus the untouched Cloud Control document, for checks that
// need a read-only attribute such as an ARN or a VpcId:
//
//	state, err := aws.ReadState[*s3.Bucket]("cloudjam-demo")
//	arn, _ := state.Attr("Arn")
//	encrypted := state.Properties.BucketEncryption != nil
func ReadState[P ResourceOf[T], T any](identifier string) (*State[T], error) {
	document, err := readResource(TypeName[P, T](), identifier)
	if err != nil {
		return nil, err
	}
	properties, err := unmarshalState[P, T](document)
	if err != nil {
		return nil, err
	}
	return &State[T]{Identifier: identifier, Properties: properties, Document: document}, nil
}

// Update applies a JSON Patch to a provisioned resource. The type parameter
// names the resource type, so no instance is needed:
//
//	err := aws.Update[*s3.Bucket]("cloudjam-demo", aws.Patch{
//		aws.Replace("/VersioningConfiguration/Status", "Enabled"),
//	})
func Update[P ResourceOf[T], T any](identifier string, patch Patch) error {
	typeName := TypeName[P, T]()
	if len(patch) == 0 {
		return fmt.Errorf("update %s %q: patch is empty", typeName, identifier)
	}
	encoded, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("marshal patch for %s %q: %w", typeName, identifier, err)
	}
	if _, err := host.UpdateResource(api.UpdateResourceInput{
		Type:       typeName,
		Identifier: identifier,
		Patch:      string(encoded),
	}); err != nil {
		return fmt.Errorf("update %s %q: %w", typeName, identifier, err)
	}
	return nil
}

// Delete removes a provisioned resource.
//
//	err := aws.Delete[*s3.Bucket]("cloudjam-demo")
func Delete[P ResourceOf[T], T any](identifier string) error {
	typeName := TypeName[P, T]()
	if _, err := host.DeleteResource(api.DeleteResourceInput{
		Type:       typeName,
		Identifier: identifier,
	}); err != nil {
		return fmt.Errorf("delete %s %q: %w", typeName, identifier, err)
	}
	return nil
}

// List returns every resource of a type in the account, keyed by primary
// identifier. It is the only call that hands identifiers back, which is how you
// find resources AWS names itself (vpc-…, i-…, sg-…):
//
//	vpcs, err := aws.List[*ec2.VPC]()
//	for id, vpc := range vpcs { ... }
func List[P ResourceOf[T], T any]() (map[string]P, error) {
	typeName := TypeName[P, T]()
	states, err := listResources(typeName)
	if err != nil {
		return nil, err
	}
	resources := make(map[string]P, len(states))
	for identifier, state := range states {
		resource, err := unmarshalState[P, T]([]byte(state))
		if err != nil {
			return nil, fmt.Errorf("list %s: %q: %w", typeName, identifier, err)
		}
		resources[identifier] = resource
	}
	return resources, nil
}

// Exists reports whether a resource is still present — the usual way to score
// "did the player delete this".
//
// It lists rather than reads, because the host surfaces a failed read as an
// opaque error, leaving no way to tell "gone" from "the call broke".
func Exists[P ResourceOf[T], T any](identifier string) (bool, error) {
	states, err := listResources(TypeName[P, T]())
	if err != nil {
		return false, err
	}
	_, ok := states[identifier]
	return ok, nil
}

func createResource(typeName string, desired json.RawMessage) (string, error) {
	if len(desired) == 0 {
		desired = json.RawMessage("{}")
	}
	out, err := host.CreateResource(api.CreateResourceInput{
		Type:    typeName,
		Desired: string(desired),
	})
	if err != nil {
		return "", fmt.Errorf("create %s: %w", typeName, err)
	}
	return out.Identifier, nil
}

func readResource(typeName, identifier string) ([]byte, error) {
	out, err := host.ReadResource(api.ReadResourceInput{
		Type:       typeName,
		Identifier: identifier,
	})
	if err != nil {
		return nil, fmt.Errorf("read %s %q: %w", typeName, identifier, err)
	}
	if out.State == "" {
		return nil, fmt.Errorf("read %s %q: host returned no state", typeName, identifier)
	}
	return []byte(out.State), nil
}

func listResources(typeName string) (map[string]string, error) {
	out, err := host.ListResource(api.ListResourceInput{Type: typeName})
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", typeName, err)
	}
	return out.Resources, nil
}
