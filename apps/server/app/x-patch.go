package app

import (
	"fmt"
	"strings"

	"github.com/protobuf-orm/protobuf-orm/graph"
	"github.com/protobuf-orm/protobuf-orm/ormpb"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"
	"google.golang.org/protobuf/compiler/protogen"
)

func (w *fileWork) xPatch() {
	name_x := w.Ident.GoName
	x := w.ent + "/" + protogen.GoImportPath(strings.ToLower(name_x))

	w.P("func (s ", name_x, "ServiceServer) Patch(",
		/* */ "ctx ", work.PkgContext.Ident("Context"), ",",
		/* */ "req *", w.Src.GoImportPath.Ident(name_x+"PatchRequest"),
		") (*", w.Ident, ", error) {")

	ver := w.Entity.GetVersionField()
	if ver != nil {
		w.P(`	is_force := req.GetDateUpdatedForce()`)
		w.P(`	if !req.HasDateUpdated() && !is_force {`)
		w.Pf(`		return nil, status.Errorf(codes.InvalidArgument, "version not given: %%s", %q)`, ver.Name())
		w.P(`	}`)
		w.P(``)
	}

	w.P("	p, err := ", name_x, "Pick(req.GetTarget())")
	w.P("	if err != nil {")
	w.P("		return nil, err")
	w.P("	}")
	w.P("")
	w.P("	q := s.Db.", name_x, ".Update().Where(p)")
	if ver != nil {
		ver_name := work.Name(ver.Name())
		w.P("	if !is_force {")
		w.Pf("		q.Where(%s(req.Get%s().AsTime()))", x.Ident(ver_name.Ent()+"EQ"), ver_name.Ent())
		w.P("	}")
	}
	for p := range w.Entity.Props() {
		if p.IsImmutable() {
			continue
		}

		name := work.Name(p.Name())
		if p.IsNullable() {
			w.P("	if req.Get", name.Go(), "Null() {")
			w.P("		q.Clear", name.Ent(), "()")
			if p.IsOptional() {
				w.Pf("	} else")
			} else {
				w.P("	}")
			}
		}

		u := "req.Get" + name.Go() + "()"
		if p == ver {
			// Do nothing.
		} else if graph.IsCollection(p) {
			w.P("	if u := ", u, "; len(u) > 0 {")
			u = "u"
		} else {
			w.P("	if req.Has", name.Go(), "() {")
		}

		switch p := p.(type) {
		case graph.Field:
			set := func(v string) {
				w.P("	q.Set", name.Ent(), "(", v, ")")
			}

			t := p.Type()
			if p.IsVersion() {
				now := work.PkgTime.Ident("Now")
				w.Pf("	if is_force && req.Has%s() {", name.Ent())
				set(fmt.Sprintf("req.Get%s().AsTime()", name.Ent()))
				w.P("	} else {")
				set(fmt.Sprintf("%s().UTC()", w.QualifiedGoIdent(now)))
				w.P("	}")
				continue
			}

			switch t {
			case ormpb.Type_TYPE_ENUM:
				if p.IsList() {
					set(u)
				} else {
					set(fmt.Sprintf("int32(%s)", u))
				}
			case ormpb.Type_TYPE_UUID:
				w.P("if v, err := ", work.PkgGoogleUuid.Ident("FromBytes"), "(", u, "); err != nil {")
				w.P("	return nil, ", work.PkgGrpcStatus.Ident("Errorf"), "(", work.PkgGrpcCodes.Ident("InvalidArgument"), ", \"", name, ": %s\", err)")
				w.P("} else {")
				set("v")
				w.P("}")
			case ormpb.Type_TYPE_TIME:
				set(u + ".AsTime()")
			default:
				set(u)
			}

		case graph.Edge:
			w.P("if id, err := ", work.Name(p.Target().Name()).Go(), "GetKey(ctx, s.Db, ", u, ")", "; err != nil {")
			w.P("	return nil, err")
			w.P("} else {")
			w.P("	q.Set", name.Ent(), "ID(id)")
			w.P("}")

		default:
			panic("unknown type of graph prop")
		}
		w.P("}")
	}
	w.P("")
	w.P("if n, err := q.Save(ctx); err != nil {")
	w.P("	return nil, err")
	w.P("} else if n == 0 {")
	if ver == nil {
		w.P(`	return nil, status.Errorf(codes.NotFound, "not found")`)
	} else {
		w.P(`	if is_force {`)
		w.P(`		return nil, status.Errorf(codes.NotFound, "not found")`)
		w.P(`	} else {`)
		w.Pf(`		return nil, status.Errorf(codes.FailedPrecondition, "version not matched: %%s", %q)`, ver.Name())
		w.P(`	}`)
	}
	w.P("}")
	w.P("")
	// https://github.com/ent/ent/issues/2600
	w.P("return s.Get(ctx, req.GetTarget().Pick())")
	w.P("}")
	w.P("")
}
