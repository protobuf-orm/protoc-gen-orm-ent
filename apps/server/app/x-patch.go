package app

import (
	"github.com/protobuf-orm/protobuf-orm/graph"
	"github.com/protobuf-orm/protobuf-orm/ormpb"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// xPatch emits Patch, which converts its request into a patch document and
// hands it to the same path Apply uses.
//
// Patch used to be a ladder: one `if req.HasX()` and one q.SetX per prop, a
// second implementation of everything Apply already does, and for an entity
// with two hundred columns it was hundreds of lines that no test covered. The
// request is not a second update language though -- it is a fixed document,
// one assign per field the caller set -- so it converts, and what is left here
// is the part a document genuinely cannot carry: resolving an edge's Ref to a
// key, which is a query, and the refusal when the version is missing, which is
// the absence of a precondition rather than a document at all.
func (w *fileWork) xPatch() {
	name_x := w.Ident.GoName
	v := lowerFirst(name_x)

	w.P("func (s ", name_x, "ServiceServer) Patch(",
		/* */ "ctx ", work.PkgContext.Ident("Context"), ",",
		/* */ "req *", w.Src.GoImportPath.Ident(name_x+"PatchRequest"),
		") (*", w.Ident, ", error) {")

	w.Pf("	doc, err := %s(%sOrmEntity, req.ProtoReflect(), ",
		w.QualifiedGoIdent(work.PkgOrmPatch.Ident("FromPatchRequest")), v)
	w.xPatchResolver()
	w.P(")")
	w.P("	if err != nil {")
	// A resolver's error is already a status -- usually NotFound for a Ref
	// that names nothing -- and saying it again in other words would lose the
	// code. Everything else is the request's fault except a layout mismatch,
	// which is this generator's.
	w.P("		if _, ok := ", work.PkgGrpcStatus.Ident("FromError"), "(err); ok {")
	w.P("			return nil, err")
	w.P("		}")
	w.Pf("		if %s(err, %s) {",
		w.QualifiedGoIdent(work.PkgErrors.Ident("Is")),
		w.QualifiedGoIdent(work.PkgOrmPatch.Ident("ErrRequestLayout")))
	w.Pf("			return nil, %s(%s, \"%%s\", err)",
		w.QualifiedGoIdent(work.PkgGrpcStatus.Ident("Errorf")),
		w.QualifiedGoIdent(work.PkgGrpcCodes.Ident("Internal")))
	w.P("		}")
	w.Pf("		if %s(err, %s) {",
		w.QualifiedGoIdent(work.PkgErrors.Ident("Is")),
		w.QualifiedGoIdent(work.PkgOrmPatch.Ident("ErrUnsupported")))
	w.Pf("			return nil, %s(%s, \"%%s\", err)",
		w.QualifiedGoIdent(work.PkgGrpcStatus.Ident("Errorf")),
		w.QualifiedGoIdent(work.PkgGrpcCodes.Ident("Unimplemented")))
	w.P("		}")
	w.Pf("		return nil, %s(%s, \"%%s\", err)",
		w.QualifiedGoIdent(work.PkgGrpcStatus.Ident("Errorf")),
		w.QualifiedGoIdent(work.PkgGrpcCodes.Ident("InvalidArgument")))
	w.P("	}")
	w.P("")

	// A request that asks for nothing converts to no document, which is not the
	// same as an empty one. apply takes it: for a versioned entity it is still
	// the version stamp, which is what `_force` alone has always meant.
	w.P("	return s.apply(ctx, req.GetRef(), doc)")
	w.P("}")
	w.P("")
}

// xPatchResolver emits the EdgeResolver literal, or `nil` when the entity has
// no edge a request can repoint.
//
// A document carries an edge as the target's key, and the request carries a
// Ref, which may name the target by any unique field. Closing that gap is a
// query, so it cannot happen inside the converter -- ormpatch never reads
// storage -- and it lands here, where the client is.
func (w *fileWork) xPatchResolver() {
	eds := []graph.Edge{}
	for p := range graph.PatchProps(w.Entity) {
		ed, ok := p.(graph.Edge)
		if !ok || ed.IsList() {
			// A repeated edge has no column, and the converter refuses it
			// before it would ask for a key.
			continue
		}
		eds = append(eds, ed)
	}
	if len(eds) == 0 {
		w.Pf("nil")
		return
	}

	ident_v := w.QualifiedGoIdent(work.PkgProtoReflect.Ident("Value"))
	w.Pf("func(ed %s, ref %s) (%s, error) {\n",
		w.QualifiedGoIdent(work.PkgOrmGraph.Ident("Edge")),
		w.QualifiedGoIdent(work.PkgProtoReflect.Ident("Message")),
		ident_v)
	w.P("		switch ed.Number() {")
	for _, ed := range eds {
		t := ed.Target()
		w.Pf("		case %d:", ed.Number())
		w.P("			k, err := ", t.Name(), "GetKey(ctx, s.Db, ref.Interface().(*",
			w.Src.GoImportPath.Ident(t.Name()+"Ref"), "))")
		w.P("			if err != nil {")
		w.P("				return ", ident_v, "{}, err")
		w.P("			}")
		w.P("			return ", w.xKeyValueOf("k", t.Key()), ", nil")
	}
	w.P("		}")
	// Unreachable: the converter only asks about edges the entity declares.
	w.Pf("		return %s{}, %s(%s, \"no key resolver for edge: %%s\", ed.Name())\n",
		ident_v,
		w.QualifiedGoIdent(work.PkgGrpcStatus.Ident("Errorf")),
		w.QualifiedGoIdent(work.PkgGrpcCodes.Ident("Internal")))
	w.Pf("	}")
}

// xKeyValueOf renders a Go key value as the protoreflect.Value the converter
// encodes against the target key's own descriptor -- raw bytes for a UUID, not
// its text.
func (w *fileWork) xKeyValueOf(v string, k graph.Field) string {
	of := func(name string, arg string) string {
		return w.QualifiedGoIdent(work.PkgProtoReflect.Ident(name)) + "(" + arg + ")"
	}
	if k.Type() == ormpb.Type_TYPE_UUID {
		return of("ValueOfBytes", v+"[:]")
	}
	switch k.Descriptor().Kind() {
	case protoreflect.BytesKind:
		return of("ValueOfBytes", v)
	case protoreflect.StringKind:
		return of("ValueOfString", v)
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return of("ValueOfInt32", v)
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return of("ValueOfInt64", v)
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return of("ValueOfUint32", v)
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return of("ValueOfUint64", v)
	default:
		panic("not implemented: key of kind " + k.Descriptor().Kind().String())
	}
}
