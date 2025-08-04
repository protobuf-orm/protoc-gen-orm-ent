package app

import (
	"github.com/ettle/strcase"
	"github.com/protobuf-orm/protobuf-orm/graph"
	"github.com/protobuf-orm/protobuf-orm/ormpb"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/ent"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"
)

func (w *fileWork) xAdd() {
	name := w.Ident.GoName
	w.P("func (s ", name, "ServiceServer) Add(",
		/* */ "ctx ", work.PkgContext.Ident("Context"), ",",
		/* */ "req *", w.Src.GoImportPath.Ident(name+"AddRequest"),
		") (*", w.Src.GoImportPath.Ident(name), ", error) {")
	w.P("q := s.Db.", name, ".Create()")
	for p := range w.Entity.Props() {
		name := string(p.FullName().Name())
		name_go := strcase.ToPascal(name)
		name_ent := ent.Pascal(name)

		if p.IsOptional() {
			w.P("if req.Has", name_go, "() {")
		}
		switch p_ := p.(type) {
		case graph.Field:
			t := p_.Type()
			switch t {
			case ormpb.Type_TYPE_UUID:
				w.P("if v, err := ", work.PkgGoogleUuid.Ident("FromBytes"), "(req.Get", name_go, "()); err != nil {")
				w.P("	return nil, ", work.PkgGrpcStatus.Ident("Errorf"), "(", work.PkgGrpcCodes.Ident("InvalidArgument"), ", \"", name, ": %s\", ", "err)")
				w.P("} else {")
				w.P("	q.Set", name_ent, "(v)")
				w.P("}")
			case ormpb.Type_TYPE_TIME:
				w.P("q.Set", name_ent, "(req.Get", name_go, "().AsTime())")
			default:
				w.P("q.Set", name_ent, "(req.Get", name_go, "())")
			}

		case graph.Edge:
			// edges = append(edges, name)
		default:
			panic("unknown type of graph prop")
		}
		if p.IsOptional() {
			w.P("}")
		}
	}
	w.P("")
	w.P("v, err := q.Save(ctx)")
	w.P("if err != nil {")
	w.P("	return nil, err")
	w.P("}")
	w.P("")
	w.P("return v.Proto(), nil")
	w.P("}")
	w.P("")
}
