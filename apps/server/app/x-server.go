package app

func (w *fileWork) xServer() {
	name := w.Ident.GoName + "ServiceServer"
	w.P("type ", name, " struct {")
	w.P("	Db *", w.ent.Ident("Client"))
	w.P("")
	w.P("	// Rec is told about every write this server makes, and nothing is told")
	w.P("	// if it is nil. See [Recorder].")
	w.P("	Rec Recorder")
	w.P("")
	w.P("	", w.Src.GoImportPath.Ident("Unimplemented"+name))
	w.P("}")
	w.P("")
	// The options are the store's, and they are taken here for one reason: a
	// server built without them records nothing, silently, which is the one
	// failure of this feature nobody would notice. Whoever builds a service
	// server by hand should be able to say what it reports to in the same
	// words the store does.
	w.P("// New", name, " answers with a server that runs its queries with `db`.")
	w.P("//")
	w.P("// It takes the options of [Server] so that what is built here can be told")
	w.P("// where to report its writes. Built without that, it reports nowhere.")
	w.P("func New", name, "(db *", w.ent.Ident("Client"), ", opts ...Option) ",
		w.Src.GoImportPath.Ident(name), "{")
	w.P("	s := Server{Db: db}")
	w.P("	for _, opt := range opts {")
	w.P("		opt(&s)")
	w.P("	}")
	w.P("	return ", name, "{Db: s.Db, Rec: s.Rec}")
	w.P("}")
	w.P("")
}
