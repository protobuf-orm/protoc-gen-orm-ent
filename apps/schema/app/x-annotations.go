package app

import (
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"
)

func xAnnotations(w *work.FileWork) {
	w.P("func (", w.Ident.GoName, ")", "Annotations() []", work.PkgSchema.Ident("Annotation"), " {")
	w.P("	return []", work.PkgSchema.Ident("Annotation"), "{")
	w.P("		", work.PkgEntSql.Ident("Annotation"), "{Table: \"", work.TableOf(w.Entity), "\"},")
	w.P("	}")
	w.P("}")
}
