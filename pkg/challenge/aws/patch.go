package aws

import "encoding/json"

// PatchOp is a single RFC 6902 JSON Patch operation. Cloud Control's
// UpdateResource applies a patch document (an ordered list of these ops) against
// a resource's current state, so mutating a provisioned resource is expressed as
// a sequence of Add / Replace / Remove operations rather than a full re-submit.
//
// Paths are JSON Pointers into the resource's properties, e.g.
// "/BucketEncryption" or "/Tags/0/Value".
type PatchOp struct {
	Op    string
	Path  string
	Value any
}

// Replace overwrites the value at path. Use it to change an existing property.
func Replace(path string, value any) PatchOp {
	return PatchOp{Op: "replace", Path: path, Value: value}
}

// Add sets the value at path, creating it if absent (or inserting into a list).
func Add(path string, value any) PatchOp {
	return PatchOp{Op: "add", Path: path, Value: value}
}

// Remove deletes the value at path.
func Remove(path string) PatchOp {
	return PatchOp{Op: "remove", Path: path}
}

// MarshalJSON emits the operation. The "value" member is only meaningful for
// add/replace, and must be omitted entirely for remove — but it must NOT be
// dropped for a legitimate falsy value (false, 0, ""), so a plain omitempty tag
// would be wrong. Hence the explicit split.
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
