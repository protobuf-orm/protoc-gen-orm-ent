package work

import (
	"fmt"

	"github.com/protobuf-orm/protobuf-orm/graph"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type Work struct {
	// Full name of message (e.g. entity) -> Go import path
	Imports map[protoreflect.FullName]protogen.GoImportPath
}

func NewWork() *Work {
	return &Work{
		Imports: map[protoreflect.FullName]protogen.GoImportPath{},
	}
}

type FileWork struct {
	Root   *Work
	Entity graph.Entity
	Ident  protogen.GoIdent

	Src *protogen.File
	*protogen.GeneratedFile

	Deferred []func()
}

func NewFileWork(root *Work, src *protogen.File, entity graph.Entity, out *protogen.GeneratedFile) *FileWork {
	root.Imports[entity.FullName()] = src.GoImportPath

	fw := &FileWork{
		Root:   root,
		Entity: entity,
		Ident:  src.GoImportPath.Ident(string(entity.FullName().Name())),

		Src:           src,
		GeneratedFile: out,

		Deferred: []func(){},
	}

	return fw
}

func (w *FileWork) Pf(format string, vs ...any) {
	ws := make([]any, len(vs))
	for i, v := range vs {
		switch v := v.(type) {
		case protogen.GoIdent:
			ws[i] = w.GeneratedFile.QualifiedGoIdent(v)
		default:
			ws[i] = v
		}
	}

	fmt.Fprintf(w, format, ws...)
}
