package aws

import "encoding/json"

// Patch is an ordered list of RFC 6902 operations. Cloud Control mutates a
// resource by applying a patch to its current state rather than by re-submitting
// the whole desired state, so every change is expressed as Add / Replace /
// Remove:
//
//	aws.Patch{
//		aws.Replace("/VersioningConfiguration/Status", "Enabled"),
//		aws.Add("/Tags/-", tags.Tag{Key: "cloudjam", Value: "locked"}),
//	}
type Patch []PatchOp

// PatchOp is a single RFC 6902 operation. Paths are JSON Pointers into the
// resource's properties, e.g. "/BucketEncryption" or "/Tags/0/Value"; "/Tags/-"
// appends to a list.
type PatchOp struct {
	Op    string
	Path  string
	Value any
}

// Replace overwrites the value at path. Values are marshalled with
// encoding/json, so goformation property structs can be passed directly.
func Replace(path string, value any) PatchOp {
	return PatchOp{Op: "replace", Path: path, Value: value}
}

// Add sets the value at path, creating it if absent or inserting into a list.
func Add(path string, value any) PatchOp {
	return PatchOp{Op: "add", Path: path, Value: value}
}

// Remove deletes the value at path.
func Remove(path string) PatchOp {
	return PatchOp{Op: "remove", Path: path}
}

// MarshalJSON emits the operation. "value" is meaningless for remove and must be
// omitted entirely — but it must NOT be dropped for a legitimate falsy value
// (false, 0, ""), which an omitempty tag would do, hence the explicit split.
func (p PatchOp) MarshalJSON() ([]byte, error) {
	if p.Op == "remove" {
		return json.Marshal(struct {
			Op   string `json:"op"`
			Path string `json:"path"`
		}{p.Op, p.Path})
	}
	return json.Marshal(struct {
		Op    string `json:"op"`
		Path  string `json:"path"`
		Value any    `json:"value"`
	}{p.Op, p.Path, p.Value})
}
