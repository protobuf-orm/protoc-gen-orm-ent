package app

import (
	"github.com/protobuf-orm/protobuf-orm/ormpb"
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

func (w *fileWork) xGetKey() {
	k := w.Entity.Key()
	t := w.EntTypeOf(k)

	name := w.Entity.Name()
	w.P("func ", name, "GetKey(",
		/* */ "ctx ", work.IdentContext, ", ",
		/* */ "db *", w.ent.Ident("Client"), ", ",
		/* */ "ref *", w.Src.GoImportPath.Ident(name+"Ref"),
		") (", t, ", error) {")
	w.P("	var z ", t)

	name_k := work.Name(k.Name())
	w.P("	if ref.Has", name_k.Go(), "() {")
	switch k.Type() {
	case ormpb.Type_TYPE_UUID:
		w.P("		if v, err := ", work.PkgGoogleUuid.Ident("FromBytes"), "(ref.Get", name_k.Go(), "()); err != nil {")
		w.P("			return z, ", work.PkgGrpcStatus.Ident("Errorf"), "(", work.PkgGrpcCodes.Ident("InvalidArgument"), ", \"", name_k.Prop(), ": %s\", ", "err)")
		w.P("		} else {")
		w.P("			return v, nil")
		w.P("		}")
	default:
		w.P("		return ref.Get", name_k.Go(), "(), nil")
	}
	w.P("	}")
	w.P("")

	w.P("	p, err := ", name, "Pick(ref)")
	w.P("	if err != nil {")
	w.P("		return z, nil")
	w.P("	}")
	w.P("")

	w.P("	v, err := db.", name, ".Query().Where(p).OnlyID(ctx)")
	w.P("	if err != nil {")
	w.P("		return z, err")
	w.P("	}")
	w.P("")
	w.P("	return v, nil")
	w.P("}")
	w.P("")
}
