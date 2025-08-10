package app

import (
	"fmt"

	"github.com/protobuf-orm/protobuf-orm/graph"
	"github.com/protobuf-orm/protobuf-orm/ormpb"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"
)

func xFields(w *work.FileWork) {
	w.P("func (", w.Ident.GoName, ") Fields() []", work.PkgEnt.Ident("Field"), " {")
	w.P("	return []", work.PkgEnt.Ident("Field"), "{")
	for p := range w.Entity.Props() {
		p_, ok := p.(graph.Field)
		if !ok {
			continue
		}

		t := p_.Type()
		builder := entField(t)
		ctor := ""
		switch t {
		case ormpb.Type_TYPE_UUID:
			ctor = w.QualifiedGoIdent(work.PkgGoogleUuid.Ident("UUID")) + "{}"
		case ormpb.Type_TYPE_JSON:
			s := p_.Shape()
			switch s_ := s.(type) {
			case graph.MessageShape:
				pkg, ok := w.Root.Imports[s_.FullName]
				if !ok {
					panic("import path for the entity must be exist")
				}

				ctor = "&" + w.QualifiedGoIdent(pkg.Ident(string(s_.FullName.Name()))) + "{}"
			case graph.MapShape:
				ctor = w.GoType(t, s) + "{}"
			default:
				panic("shape not implemented")
			}
		}

		name := string(p.FullName().Name())
		fmt.Fprintf(w, "		%s(%q", w.QualifiedGoIdent(builder), name)
		if ctor != "" {
			fmt.Fprintf(w, ", %s", ctor)
		}
		fmt.Fprint(w, ")")
		if p.IsUnique() {
			w.P(".")
			fmt.Fprint(w, "			Unique()")
		}
		if p.IsNullable() {
			w.P(".")
			fmt.Fprint(w, "			Nillable()")
		}
		if p.IsImmutable() {
			w.P(".")
			fmt.Fprint(w, "			Immutable()")
		}
		if p.IsOptional() && p != w.Entity.Key() {
			w.P(".")
			fmt.Fprint(w, "			Optional()")
		}
		w.P(",")
	}
	w.P("	}")
	w.P("}")
	w.P("")
}
