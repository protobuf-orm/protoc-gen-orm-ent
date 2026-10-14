package app

import (
	"github.com/protobuf-orm/protobuf-orm/graph/gogen"
	"github.com/protobuf-orm/protobuf-orm/ormpb"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"
)

func (w *fileWork) xGet() {
	name_x := w.Entity.Name()
	w.P("func (s ", name_x, "ServiceServer) Get(",
		/* */ "ctx ", work.IdentContext, ", ",
		/* */ "req *", w.Src.GoImportPath.Ident(name_x+"GetRequest"),
		") (*", w.Ident, ", error) {")
	w.P("	p, err := ", name_x, "Pick(req.GetRef())")
	w.P("	if err != nil {")
	w.P("		return nil, err")
	w.P("	}")
	// A row out of scope is one this query does not match, so it is reported
	// below as NotFound rather than as a refusal -- which is the answer to
	// give: that a row exists is itself something a caller who may not read it
	// should not be told.
	w.P("	p, err = s.narrow(ctx, p)")
	w.P("	if err != nil {")
	w.P("		return nil, err")
	w.P("	}")
	w.P("")
	w.P("	q := s.Db.", name_x, ".Query().Where(p)")
	w.P("	", name_x, "SelectInit(q, req.GetSelect())")
	w.P("")
	w.P("	v, err := q.Only(ctx)")
	w.P("	if err != nil {")
	w.P("		if ", w.ent.Ident("IsNotFound"), "(err) {")
	w.P("			return nil, ", work.PkgGrpcStatus.Ident("Error"), "(", work.PkgGrpcCodes.Ident("NotFound"), ", \"", name_x, " not found\")")
	w.P("		}")
	w.P("		return nil, err")
	w.P("	}")
	w.P("	return v.Proto(), nil")
	w.P("}")
	w.P("")
	// Lock the row before reading the preimage so concurrent writers cannot
	// make the recorded Previous differ from the row this mutation replaced.
	w.P("func (s ", name_x, "ServiceServer) snapshot(ctx ", work.IdentContext,
		", p ", w.entPkg().Ident(name_x), ") (*", w.ent.Ident(name_x), ", error) {")
	w.P("	q := s.Db.", name_x, ".Query().Where(p)")
	w.P("	", name_x, "SelectInit(q, nil)")
	w.P("	q.Modify(func(q *", work.PkgEntSQLBuilder.Ident("Selector"), ") {")
	w.P("		if s.Db.Dialect() != ", work.PkgEntDialect.Ident("SQLite"), " { q.ForUpdate() }")
	w.P("	})")
	w.P("	return q.Only(ctx)")
	w.P("}")

	w.P("func (s ", name_x, "ServiceServer) image(v *", w.Ident, ") *", w.Ident, " {")
	w.P("if s.Rec == nil || v == nil { return nil }")
	w.P("out := ", work.PkgProto.Ident("Clone"), "(v).(*", w.Ident, ")")
	if w.Entity.HasEdges() {
		w.P("m := out.ProtoReflect()")
		for edge := range w.Entity.Edges() {
			w.P("{ f := m.Descriptor().Fields().ByNumber(", edge.Number(), "); if m.Has(f) {")
			w.P("trim := func(r ", work.PkgProtoReflect.Ident("Message"), ") {r.Range(func(f ", work.PkgProtoReflect.Ident("FieldDescriptor"), ", _ ", work.PkgProtoReflect.Ident("Value"), ") bool {if f.Number()!=", edge.Target().Key().Number(), " {r.Clear(f)};return true})}")
			if edge.IsList() {
				w.P("vs:=m.Get(f).List();for i:=0;i<vs.Len();i++ {trim(vs.Get(i).Message())}")
			} else {
				w.P("trim(m.Get(f).Message())")
			}
			w.P("}}")
		}
	}
	w.P("return out")
	w.P("}")

}

func (w *fileWork) xGetKey() {
	k := w.Entity.Key()
	t := gogen.GoTypeOf(k, w.QualifiedGoIdent)

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
		w.P("		if v, err := ", work.PkgEntUuid.Ident("FromBytes"), "(ref.Get", name_k.Go(), "()); err != nil {")
		w.P("			return z, ", work.PkgGrpcStatus.Ident("Errorf"), "(", work.PkgGrpcCodes.Ident("InvalidArgument"), ", \"", name_k, ": %s\", ", "err)")
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
	w.P("		return z, err")
	w.P("	}")
	w.P("")

	w.P("	v, err := db.", name, ".Query().Where(p).OnlyId(ctx)")
	w.P("	if err != nil {")
	w.P("		if ", w.ent.Ident("IsNotFound"), "(err) {")
	w.P("			return z, ", work.PkgGrpcStatus.Ident("Error"), "(", work.PkgGrpcCodes.Ident("NotFound"), ", \"", name, " not found\")")
	w.P("		}")
	w.P("		return z, err")
	w.P("	}")
	w.P("")
	w.P("	return v, nil")
	w.P("}")
	w.P("")
}
