package sortid

import (
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
)

const sampleSize = 100

func TestUUID(t *testing.T) {
	id := New()
	_, err := uuid.Parse(id.String())
	if err != nil {
		t.Fatal(err)
	}

	ids, oldest, newest := []string{}, "", ""
	for i := range sampleSize {
		id := New().String()
		switch i {
		case 0:
			oldest = id
		case sampleSize - 1:
			newest = id
		}
		time.Sleep(time.Millisecond)
		ids = append(ids, id)
	}
	slices.Sort(ids)
	if ids[0] != newest {
		t.Fatalf("descending time sorting of uuid does not work")
	}
	if ids[sampleSize-1] != oldest {
		t.Fatalf("descending time sorting of uuid does not work")
	}
}
