package app

func (w *fileWork) xServer() {
	name := w.Ident.GoName + "ServiceServer"
	w.P("type ", name, " struct {")
	w.P("	Db *", w.ent.Ident("Client"))
	w.P("	", w.Src.GoImportPath.Ident("Unimplemented"+name))
	w.P("}")
	w.P("")
	w.P("func New", name, "(db *", w.ent.Ident("Client"), ") ", name, "{")
	w.P("	return ", name, "{Db: db}")
	w.P("}")
	w.P("")
}
