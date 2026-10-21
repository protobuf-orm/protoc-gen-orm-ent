package work

import (
	"strings"

	"github.com/protobuf-orm/ent/entc/gen"
	"github.com/protobuf-orm/protobuf-orm/graph"
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

// TableOf is the table an entity's rows are in: its name, lower case.
func TableOf(e graph.Entity) string { return strings.ToLower(e.Name()) }

// IndexKey is what an index the schema names is stored as: the entity's table
// and the index's name, `robot_page`.
//
// The table because a database keeps index names in one place for every table
// -- a PostgreSQL schema, a SQLite database -- so `page` alone would be one
// index for every entity that has one.
func IndexKey(e graph.Entity, index string) string { return TableOf(e) + "_" + index }

// MaxIndexKey is the longest [IndexKey] there may be. PostgreSQL keeps the first
// 63 bytes of a name and drops the rest without a word, so two keys longer than
// that which begin alike are one index to it.
const MaxIndexKey = 63
