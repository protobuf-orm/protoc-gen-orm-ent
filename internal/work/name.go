package work

import (
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/ent"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/strs"
)

type Name string

func (v Name) Go() string {
	return strs.GoCamelCase(string(v))
}

func (v Name) Ent() string {
	return ent.Pascal(string(v))
}
