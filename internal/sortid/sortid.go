// Package sortid creates a thin uuidv7 wrapper that generates valid uuidv7 ids that are sorted in descending time order (last in first out).
// This is primarily useful for dynamitedb partition keys that are sorted in lexical ascending order (which means with sortid, entities are listed "newest first").
package sortid

import (
	"encoding/binary"

	"github.com/google/uuid"
)

const max48Bit uint64 = (1 << 48) - 1

type UUID = uuid.UUID

func New() UUID {
	rawId, err := uuid.NewV7()
	if err != nil {
		panic(err)
	}
	buffer := make([]byte, 8)
	copy(buffer[2:8], rawId[0:6])
	ms := binary.BigEndian.Uint64(buffer)

	reversed := max48Bit - ms
	copy(rawId[0:6], binary.BigEndian.AppendUint64(nil, reversed)[2:8])
	return UUID(rawId)
}
