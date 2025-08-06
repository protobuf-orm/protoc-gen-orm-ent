package app

import (
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/ent"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"
)

func (w *fileWork) xGet() {
	name := w.Entity.Name()
	w.P("func (s ", name, "ServiceServer) Get(",
		/* */ "ctx ", work.IdentContext, ", ",
		/* */ "req *", w.Src.GoImportPath.Ident(name+"GetRequest"),
		") (*", w.Ident, ", error) {")
	w.P("	q := s.Db.", name, ".Query()")
	w.P("")
	w.P("	if p, err := ", name, "Pick(req.GetRef()); err != nil {")
	w.P("		return nil, err")
	w.P("	} else {")
	w.P("		q.Where(p)")
	w.P("	}")
	w.P("")
	w.P("	if s := req.GetSelect(); s != nil {")
	w.P("		// TODO")
	w.P("	} else {")
	for p := range w.Entity.Edges() {

		w.P("		q.With", ent.Pascal(p.Name()), "(select", p.Target().Name(), "Key)")
	}
	w.P("	}")
	w.P("")
	w.P("	v, err := q.Only(ctx)")
	w.P("	if err != nil {")
	w.P("		return nil, err")
	w.P("	}")
	w.P("	return v.Proto(), nil")
	w.P("}")
	w.P("")
}
