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
	for p := range w.Entity.Fields() {
		t := p.Type()
		name := p.Name()
		ctor := ""
		builder := entField(t)
		if p.IsList() {
			builder = work.PkgField.Ident("JSON")
		}
		if builder == work.PkgField.Ident("JSON") {
			if p.IsList() {
				ctor = "[]"
			}

			switch t {
			case ormpb.Type_TYPE_UUID:
				ctor += w.QualifiedGoIdent(work.PkgGoogleUuid.Ident("UUID")) + "{}"
			case ormpb.Type_TYPE_JSON:
				s := p.Shape()
				switch s_ := s.(type) {
				case graph.MessageShape:
					pkg := work.MustGetGoImportPath(s_.Descriptor.ParentFile())
					if p.IsList() {
						ctor = "[]*"
					} else {
						ctor = "&"
					}
					ctor += w.QualifiedGoIdent(pkg.Ident(string(s_.FullName.Name()))) + "{}"

				case graph.MapShape:
					ctor = w.GoType(t, s) + "{}"

				default:
					panic("shape not implemented")
				}
			default:
				ctor += w.GoTypeOf(p)
				if p.IsList() {
					ctor += "{}"
				}
			}
		} else if t == ormpb.Type_TYPE_UUID {
			ctor = w.QualifiedGoIdent(work.PkgGoogleUuid.Ident("UUID")) + "{}"
		}

		if ctor == "" {
			fmt.Fprintf(w, "		%s(%q)", w.QualifiedGoIdent(builder), name)
		} else {
			fmt.Fprintf(w, "		%s(%q, %s)", w.QualifiedGoIdent(builder), name, ctor)
		}

		is_key := p == w.Entity.Key()
		if p.IsUnique() {
			w.P(".")
			fmt.Fprint(w, "			Unique()")
		}
		if p.IsNullable() && !is_key && t != ormpb.Type_TYPE_JSON {
			w.P(".")
			fmt.Fprint(w, "			Nillable()")
		}
		if p.IsImmutable() {
			w.P(".")
			fmt.Fprint(w, "			Immutable()")
		}
		if p.IsOptional() && !is_key {
			w.P(".")
			fmt.Fprint(w, "			Optional()")
		}
		w.P(",")
	}
	w.P("	}")
	w.P("}")
	w.P("")
}
