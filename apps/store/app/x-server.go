package app

func (w *Work) xServer() {
	w.P("type Server struct {")
	w.P("	Db *", w.Ent.Ident("Client"))
	w.P("}")
	w.P("")

	w.P("func NewServer(db *", w.Ent.Ident("Client"), ") Server {")
	w.P("	return Server{Db: db}")
	w.P("}")
	w.P("")

	for _, v := range w.Entities {
		w.P("func (s Server) ", v.Name(), "() ", w.Package.Ident(v.Name()+"ServiceServer"), " { return New", v.Name(), "ServiceServer(s.Db) }")
	}
	w.P("")
}
