package app

func (w *fileWork) xServer() {
	name := w.Ident.GoName + "ServiceServer"
	w.P("type ", name, " struct {")
	w.P("	Db *", w.ent.Ident("Client"))
	w.P("	// Dialect is the SQL this server writes; the store's NewServer is")
	w.P("	// what refuses one nobody wrote.")
	w.P("	Dialect string")
	w.P("	", w.Src.GoImportPath.Ident("Unimplemented"+name))
	w.P("}")
	w.P("")
	w.P("func New", name, "(db *", w.ent.Ident("Client"), ", dialect string) ", w.Src.GoImportPath.Ident(name), "{")
	w.P("	return ", name, "{Db: db, Dialect: dialect}")
	w.P("}")
	w.P("")
}
