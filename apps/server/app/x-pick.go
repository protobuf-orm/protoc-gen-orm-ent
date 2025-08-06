package app

import (
	"slices"
	"strings"

	"github.com/protobuf-orm/protobuf-orm/graph"
	"github.com/protobuf-orm/protobuf-orm/ormpb"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"
	"google.golang.org/protobuf/compiler/protogen"
)

func (w *fileWork) xPick() {
	name := w.Ident.GoName
	pkg := w.Src.GoImportPath
	x := w.ent + "/" + protogen.GoImportPath(strings.ToLower(name))
	predicate := (w.ent + "/predicate").Ident(name)

	w.P("func ", name, "Pick(",
		/* */ "req *", pkg.Ident(name), "Ref",
		") (", predicate, ", error) {")
	w.P("	switch req.WhichKey() {")
	for p := range w.Entity.Props() {
		if !p.IsUnique() {
			continue
		}

		name := work.Name(p.Name())
		switch p_ := p.(type) {
		case graph.Field:
			eq := x.Ident(name.Ent() + "EQ")

			w.P("	case ", pkg.Ident(w.Ident.GoName+"Ref_"+name.Go()+"_case"), ":")
			t := p_.Type()
			switch t {
			case ormpb.Type_TYPE_UUID:
				w.P("		if v, err := ", work.PkgGoogleUuid.Ident("FromBytes"), "(req.Get", name.Go(), "()); err != nil {")
				w.P("			return nil, ", work.PkgGrpcStatus.Ident("Errorf"), "(", work.PkgGrpcCodes.Ident("InvalidArgument"), ", \"", name, ": %s\", ", "err)")
				w.P("		} else {")
				w.P("			return ", eq, "(v), nil")
				w.P("		}")
			case ormpb.Type_TYPE_TIME:
				w.P("		return ", eq, "(req.Get", name.Go(), "().AsTime()), nil")
			default:
				w.P("		return ", eq, "(req.Get", name.Go(), "()), nil")
			}
		case graph.Edge:
			panic("not implemented: pick unique edge")
		default:
			panic("unknown type of graph prop")
		}
	}
	for p := range w.Entity.Indexes() {
		if !p.IsUnique() {
			continue
		}

		name_k := work.Name(p.Name())
		w.P("	case ", pkg.Ident(w.Ident.GoName+"Ref_"+name_k.Go()+"_case"), ":")
		w.P("		k := req.Get", name_k.Go(), "()")
		w.P("		ps := make([]", predicate, ", 0, ", len(slices.Collect(p.Props())), ")")
		for p := range p.Props() {
			name := work.Name(p.Name())
			eq := x.Ident(name.Ent() + "EQ")
			switch p_ := p.(type) {
			case graph.Field:
				t := p_.Type()
				switch t {
				case ormpb.Type_TYPE_UUID:
					w.P("		if v, err := ", work.PkgGoogleUuid.Ident("FromBytes"), "(k.Get", name.Go(), "()); err != nil {")
					w.P("			return nil, ", work.PkgGrpcStatus.Ident("Errorf"), "(", work.PkgGrpcCodes.Ident("InvalidArgument"), ", \"", name_k, ".", name, ": %s\", ", "err)")
					w.P("		} else {")
					w.P("			ps = append(ps, ", eq, "(v))")
					w.P("		}")
				case ormpb.Type_TYPE_TIME:
					w.P("		return ", eq, "(k.Get", name.Go(), "().AsTime()), nil")
				default:
					w.P("		ps = append(ps, ", eq, "(k.Get", name.Go(), "()))")
				}
			case graph.Edge:
				name_target := work.Name(p_.Target().Name())
				w.P("		if p, err := ", name_target, "Pick(k.Get", name_target.Go(), "()); err != nil {")
				w.P("			return nil, ", work.PkgGrpcStatus.Ident("Errorf"), "(", work.PkgGrpcCodes.Ident("InvalidArgument"), ", \"", name_k, ".", name, ": %s\", ", "err)")
				w.P("		} else {")
				w.P("			ps = append(ps, ", x.Ident("Has"+name_target.Go()+"With"), "(p))")
				w.P("		}")
			default:
				panic("unknown type of graph prop")
			}
		}
		w.P("		return ", x.Ident("And"), "(ps...) , nil")
	}
	w.P("	default:")
	w.P("		return nil, ", work.PkgGrpcStatus.Ident("Errorf"), "(", work.PkgGrpcCodes.Ident("InvalidArgument"), ", \"unknown type of key: %s\", req.WhichKey())")
	w.P("	}")
	w.P("}")
	w.P("")
}
