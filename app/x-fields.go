package app

import (
	"fmt"

	"github.com/protobuf-orm/protobuf-orm/graph"
	"github.com/protobuf-orm/protobuf-orm/ormpb"
)

func (w *fileWork) xFields() {
	name := string(w.entity.FullName().Name())

	w.P("func (", name, ")", "Fields() []", pkgEnt.Ident("Field"), " {")
	w.P("	return []", pkgEnt.Ident("Field"), "{")
	for p := range w.entity.Props() {
		p_, ok := p.(graph.Field)
		if !ok {
			continue
		}

		t := p_.Type()
		builder := entField(t)
		ctor := ""
		switch t {
		case ormpb.Type_TYPE_UUID:
			ctor = w.QualifiedGoIdent(pkgGoogleUuid.Ident("UUID")) + "{}"
		case ormpb.Type_TYPE_JSON:
			s := p_.Shape()
			switch s_ := s.(type) {
			case graph.MessageShape:
				pkg, ok := w.root.imports[string(s_.FullName)]
				if !ok {
					panic("import path for the entity must be exist")
				}

				ctor = "&" + w.QualifiedGoIdent(pkg.Ident(string(s_.FullName.Name()))) + "{}"
			case graph.MapShape:
				ctor = w.goType(t, s) + "{}"
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
		if p.IsOptional() {
			w.P(".")
			fmt.Fprint(w, "			Optional()")
		}
		w.P(",")
	}
	w.P("	}")
	w.P("}")
	w.P("")
}
