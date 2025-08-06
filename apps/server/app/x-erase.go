package app

import (
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"
)

func (w *fileWork) xErase() {
	name := w.Entity.Name()
	w.P("func (s ", name, "ServiceServer) Erase(",
		/* */ "ctx ", work.IdentContext, ", ",
		/* */ "req *", w.Src.GoImportPath.Ident(name+"Ref"),
		") (*", work.IdentEmpty, ", error) {")
	w.P("	p, err := ", name, "Pick(req)")
	w.P("	if err != nil {")
	w.P("		return nil, err")
	w.P("	}")
	w.P("")
	w.P("	if _, err := s.Db.", name, ".Delete().Where(p).Exec(ctx); err != nil {")
	w.P("		return nil, err")
	w.P("	}")
	w.P("	return nil, nil")
	w.P("}")
	w.P("")
}
