package app

import (
	"github.com/ettle/strcase"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/ent"
)

type Name string

func (v Name) Prop() string {
	return strcase.ToSnake(string(v))
}

func (v Name) Go() string {
	return strcase.ToPascal(string(v))
}

func (v Name) Ent() string {
	return ent.Pascal(string(v))
}
