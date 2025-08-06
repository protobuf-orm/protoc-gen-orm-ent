package app

import (
	"strings"

	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/ent"
	"google.golang.org/protobuf/compiler/protogen"
)

func (w *fileWork) xSelectKey() {
	name := w.Entity.Name()

	x := w.ent + "/" + protogen.GoImportPath(strings.ToLower(name))
	w.P("func select", name, "Key(",
		/* */ "q *", w.ent.Ident(name+"Query"),
		") {")
	w.P("	q.Select(", x.Ident("Field"+ent.Pascal(w.Entity.Key().Name())), ")")
	w.P("}")
	w.P("")
}

// func (w *fileWork) xSelect() {
// 	name := w.Entity.Name()
// 	w.P("func ", name, "Select(",
// 		/* */ "q *", w.ent.Ident(name+"Query"), ", ",
// 		/* */ "m *", w.Src.GoImportPath.Ident(name+"Select"),
// 		") {")
// 	w.P("if ")
// 	w.P("}")
// }
