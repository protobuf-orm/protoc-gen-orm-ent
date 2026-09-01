package work

import (
	"github.com/protobuf-orm/ent/entc/gen"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/strs"
)

type Name string

func (v Name) Go() string {
	return strs.GoCamelCase(string(v))
}

func (v Name) Ent() string {
	return gen.Pascal(string(v))
}

// EdgeFieldSuffix is what the field holding an edge's key is called: the
// edge's name and this.
//
// The field exists because ent keeps that column either way and, unbound,
// keeps it in an unexported struct member -- so the value is in memory,
// already paid for, and unreadable from anywhere that would use it.
const EdgeFieldSuffix = "_id"

// EdgeField is that field's name for one edge, and EdgeFieldGo is what ent
// calls it in Go.
func EdgeField(edge string) string   { return edge + EdgeFieldSuffix }
func EdgeFieldGo(edge string) string { return Name(EdgeField(edge)).Ent() }
