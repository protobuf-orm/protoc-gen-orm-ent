package app

import (
	"github.com/protobuf-orm/protobuf-orm/graph"
	"github.com/protobuf-orm/protobuf-orm/ormpb"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"
)

func (w *fileWork) xPatch() {
	name := w.Ident.GoName
	w.P("func (s ", name, "ServiceServer) Patch(",
		/* */ "ctx ", work.PkgContext.Ident("Context"), ",",
		/* */ "req *", w.Src.GoImportPath.Ident(name+"PatchRequest"),
		") (*", w.Ident, ", error) {")
	w.P("	p, err := ", name, "Pick(req.GetTarget())")
	w.P("	if err != nil {")
	w.P("		return nil, err")
	w.P("	}")
	w.P("")
	w.P("	q := s.Db.", name, ".Update().Where(p)")
	for p := range w.Entity.Props() {
		if p.IsImmutable() {
			continue
		}

		name := work.Name(p.Name())
		if p.IsNullable() {
			w.P("	if req.Get", name.Go(), "Null() {")
			w.P("		q.Clear", name.Ent(), "()")
			if p.IsOptional() {
				w.Pf("	} else")
			} else {
				w.P("	}")
			}
		}

		if p.IsOptional() {
			w.P("	if req.Has", name.Go(), "() {")
		}
		switch p_ := p.(type) {
		case graph.Field:
			t := p_.Type()
			switch t {
			case ormpb.Type_TYPE_UUID:
				w.P("if v, err := ", work.PkgGoogleUuid.Ident("FromBytes"), "(req.Get", name.Go(), "()); err != nil {")
				w.P("	return nil, ", work.PkgGrpcStatus.Ident("Errorf"), "(", work.PkgGrpcCodes.Ident("InvalidArgument"), ", \"", name, ": %s\", err)")
				w.P("} else {")
				w.P("	q.Set", name.Ent(), "(v)")
				w.P("}")
			case ormpb.Type_TYPE_TIME:
				w.P("q.Set", name.Ent(), "(req.Get", name.Go(), "().AsTime())")
			default:
				w.P("q.Set", name.Ent(), "(req.Get", name.Go(), "())")
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
	w.P("if _, err := q.Save(ctx); err != nil {")
	w.P("	return nil, err")
	w.P("}")
	w.P("")
	w.P("return nil, nil")
	w.P("}")
	w.P("")
}
