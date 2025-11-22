package app

import (
	"strings"

	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"
)

func xAnnotations(w *work.FileWork) {
	w.P("func (", w.Ident.GoName, ")", "Annotations() []", work.PkgSchema.Ident("Annotation"), " {")
	w.P("	return []", work.PkgSchema.Ident("Annotation"), "{")
	w.P("		", work.PkgEntSql.Ident("Annotation"), "{Table: \"", strings.ToLower(w.Entity.Name()), "\"},")
	w.P("	}")
	w.P("}")
}
