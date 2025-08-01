package app

import (
	"context"
	"fmt"

	"github.com/protobuf-orm/protobuf-orm/graph"
	"google.golang.org/protobuf/compiler/protogen"
)

type work struct {
	// Full name of entity -> Go import path
	imports map[string]protogen.GoImportPath
}

func newWork() *work {
	return &work{
		imports: map[string]protogen.GoImportPath{},
	}
}

type fileWork struct {
	*protogen.GeneratedFile

	root   *work
	entity graph.Entity
	pkg    protogen.GoImportPath

	deferred []func()
}

func (w *work) newFileWork(file *protogen.GeneratedFile, entity graph.Entity) *fileWork {
	pkg, ok := w.imports[string(entity.FullName())]
	if !ok {
		panic("import path for the entity must be exist")
	}

	fw := &fileWork{
		GeneratedFile: file,

		root:   w,
		entity: entity,
		pkg:    pkg,

		deferred: []func(){},
	}

	return fw
}

func (w *fileWork) Pf(format string, a ...any) {
	fmt.Fprintf(w, format, a...)
}

func (w *work) run(ctx context.Context, gf *protogen.GeneratedFile, entity graph.Entity) error {
	name := string(entity.FullName().Name())
	gf.P("type ", name, " struct {")
	gf.P("	", pkgEnt.Ident("Schema"))
	gf.P("}")
	gf.P("")

	fw := w.newFileWork(gf, entity)
	fw.xFields()
	fw.xEdges()
	fw.xIndexes()

	return nil
}
