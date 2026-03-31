package model

import (
	"fmt"
	"strings"
)

type Partition string

func (p Partition) New(id string) PartitionValue {
	return PartitionValue(fmt.Sprint(p, id))
}

type PartitionValue string

func (p PartitionValue) ID(pk Partition) string {
	return strings.TrimPrefix(string(p), string(pk))
}

type Sort string

func (s Sort) New(id string) SortValue {
	return SortValue(fmt.Sprint(s, id))
}

type SortValue string

func (s SortValue) ID(sk Sort) string {
	return strings.TrimPrefix(string(s), string(sk))
}
