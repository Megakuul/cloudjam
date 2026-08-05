package api

import (
	"encoding/json"
	"fmt"

	"github.com/extism/go-pdk"
)

// call marshals in, invokes the host function, and decodes the response.
func call[In, Out any](fn func(uint64) uint64, in In) (Out, error) {
	var out Out

	data, err := json.Marshal(in)
	if err != nil {
		return out, fmt.Errorf("marshal host request: %w", err)
	}
	mem := pdk.AllocateBytes(data)
	defer mem.Free()

	offset := fn(mem.Offset())
	if offset == 0 {
		// No response payload. Legitimate for the calls whose output struct is
		// empty, so this is not an error.
		return out, nil
	}
	rmem := pdk.FindMemory(offset)
	raw := rmem.ReadBytes()
	if len(raw) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, fmt.Errorf("decode host response: %w", err)
	}
	return out, nil
}
