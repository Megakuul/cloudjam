// Package aws reads and writes cloud control resources from a challenge plugin.
//
// The resource types live in the per service subpackages and are generated from
// the aws resource schemas, so a resource is a plain struct of pointers with the
// exact cloud control property names:
//
//	id, err := aws.Create(&s3.Bucket{BucketName: aws.String("demo")})
//	bucket, err := aws.Get[*s3.Bucket](id)
//	err = aws.Patch(id, &s3.Bucket{VersioningConfiguration: &s3.VersioningConfiguration{
//		Status: aws.String("Enabled"),
//	}})
package aws

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"codeberg.org/megakuul/cloudjam/pkg/challenge/api"
)

//go:generate go run ./internal/generate -out .

// Resource is a generated cloud control resource. T is always a pointer, e.g.
// *s3.Bucket.
type Resource interface {
	CloudControlType() string
}

// Create provisions a resource and returns its primary identifier.
func Create[T Resource](resource T) (string, error) {
	desired, err := json.Marshal(resource)
	if err != nil {
		return "", err
	}
	out, err := api.CreateResource(api.CreateResourceInput{
		Type:    resource.CloudControlType(),
		Desired: string(desired),
	})
	if err != nil {
		return "", fmt.Errorf("create %s: %w", resource.CloudControlType(), err)
	}
	if out.Identifier == "" {
		return "", fmt.Errorf("create %s: no identifier returned", resource.CloudControlType())
	}
	return out.Identifier, nil
}

// Read loads a resource's live state, read only attributes included.
func Read[T Resource](identifier string) (T, error) {
	resource := empty[T]()
	out, err := api.ReadResource(api.ReadResourceInput{
		Type:       resource.CloudControlType(),
		Identifier: identifier,
	})
	if err != nil {
		return resource, fmt.Errorf("read %s %q: %w", resource.CloudControlType(), identifier, err)
	}
	if out.State == "" {
		return resource, fmt.Errorf("read %s %q: no state returned", resource.CloudControlType(), identifier)
	}
	return resource, json.Unmarshal([]byte(out.State), resource)
}

// List returns every resource of a type, keyed by primary identifier. It is how
// you find resources aws named itself (vpc-…, i-…, sg-…).
func List[T Resource]() (map[string]T, error) {
	typeName := empty[T]().CloudControlType()
	out, err := api.ListResource(api.ListResourceInput{Type: typeName})
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", typeName, err)
	}
	resources := map[string]T{}
	for identifier, state := range out.Resources {
		resource := empty[T]()
		if err := json.Unmarshal([]byte(state), resource); err != nil {
			return nil, fmt.Errorf("list %s: %q: %w", typeName, identifier, err)
		}
		resources[identifier] = resource
	}
	return resources, nil
}

// Patch applies the fields you set on changes and leaves every other field
// alone:
//
//	aws.Patch(id, &s3.Bucket{PublicAccessBlockConfiguration: &s3.BucketPublicAccessBlockConfiguration{
//		BlockPublicAcls: aws.Bool(true),
//	}})
//
// becomes [{"op":"add","path":"/PublicAccessBlockConfiguration/BlockPublicAcls","value":true}].
func Patch[T Resource](identifier string, changes T) error {
	typeName := changes.CloudControlType()
	operations := patch(reflect.ValueOf(changes), "")
	if len(operations) == 0 {
		return fmt.Errorf("patch %s %q: no fields set", typeName, identifier)
	}
	document, err := json.Marshal(operations)
	if err != nil {
		return err
	}
	if _, err := api.UpdateResource(api.UpdateResourceInput{
		Type:       typeName,
		Identifier: identifier,
		Patch:      string(document),
	}); err != nil {
		return fmt.Errorf("patch %s %q: %w", typeName, identifier, err)
	}
	return nil
}

// Delete removes a resource.
func Delete[T Resource](identifier string) error {
	typeName := empty[T]().CloudControlType()
	if _, err := api.DeleteResource(api.DeleteResourceInput{
		Type:       typeName,
		Identifier: identifier,
	}); err != nil {
		return fmt.Errorf("delete %s %q: %w", typeName, identifier, err)
	}
	return nil
}

type operation struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value"`
}

// patch walks the fields that are set and turns them into rfc 6902 operations.
// "add" rather than "replace" because it works whether or not the property is
// already there.
func patch(value reflect.Value, path string) []operation {
	value = reflect.Indirect(value)
	operations := []operation{}

	for i := range value.NumField() {
		field := value.Field(i)
		if field.IsZero() {
			continue
		}
		name, _, _ := strings.Cut(value.Type().Field(i).Tag.Get("json"), ",")
		at := path + "/" + name

		if field.Kind() == reflect.Pointer && field.Elem().Kind() == reflect.Struct {
			operations = append(operations, patch(field, at)...)
			continue
		}
		operations = append(operations, operation{Op: "add", Path: at, Value: field.Interface()})
	}
	return operations
}

// empty builds the zero resource of a pointer type, so the generic calls can
// reach CloudControlType without being handed an instance.
func empty[T Resource]() T {
	return reflect.New(reflect.TypeFor[T]().Elem()).Interface().(T)
}
