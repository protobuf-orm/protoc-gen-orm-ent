package app

import (
	"github.com/protobuf-orm/protobuf-orm/graph"
	"google.golang.org/protobuf/compiler/protogen"
)

type Work struct {
	*protogen.GeneratedFile

	Entities []graph.Entity
	Package  protogen.GoImportPath
	Ent      protogen.GoImportPath
}
