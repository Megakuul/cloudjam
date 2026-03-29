package model

import (
	"fmt"
	"strings"
)

type Partition string

func (p Partition) New(id string) Partition {
	return Partition(fmt.Sprint(p, id))
}

func (p Partition) ID(pk string) string {
	return strings.TrimPrefix(pk, string(p))
}

type Sort string

func (s Sort) New(id ...string) Sort {
	return Sort(fmt.Sprint(s, id))
}

func (s Sort) ID(sk string) string {
	return strings.TrimPrefix(sk, string(s))
}
