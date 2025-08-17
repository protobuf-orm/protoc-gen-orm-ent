package app

import (
	"fmt"

	"github.com/protobuf-orm/protobuf-orm/graph"
	"github.com/protobuf-orm/protobuf-orm/ormpb"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"
)

func (w *fileWork) xAdd() {
	name := w.Ident.GoName
	w.P("func (s ", name, "ServiceServer) Add(",
		/* */ "ctx ", work.PkgContext.Ident("Context"), ",",
		/* */ "req *", w.Src.GoImportPath.Ident(name+"AddRequest"),
		") (*", w.Src.GoImportPath.Ident(name), ", error) {")
	w.P("	q := s.Db.", name, ".Create()")
	for p := range w.Entity.Props() {
		name := work.Name(p.Name())
		u := "req.Get" + name.Go() + "()"

		if p.IsOptional() {
			if graph.IsCollection(p) {
				w.P("	if u := ", u, "; len(u) > 0 {")
				u = "u"
			} else {
				w.P("	if req.Has", name.Go(), "() {")
			}
		}

		switch p := p.(type) {
		case graph.Field:
			set := func(v string) {
				w.P("	q.Set", name.Ent(), "(", v, ")")
			}

			t := p.Type()
			switch t {
			case ormpb.Type_TYPE_ENUM:
				if p.IsList() {
					// Repeated field is stored as JSON in Ent
					// so no type conversion is needed.
					set(u)
				} else {
					set(fmt.Sprintf("int32(%s)", u))
				}
			case ormpb.Type_TYPE_UUID:
				w.P("	if v, err := ", work.PkgGoogleUuid.Ident("FromBytes"), "(", u, "); err != nil {")
				w.P("		return nil, ", work.PkgGrpcStatus.Ident("Errorf"), "(", work.PkgGrpcCodes.Ident("InvalidArgument"), ", \"", name, ": %s\", err)")
				w.P("	} else {")
				set("v")
				w.P("	}")
			case ormpb.Type_TYPE_TIME:
				set(u + ".AsTime()")
			default:
				set(u)
			}

		case graph.Edge:
			w.P("	if k, err := ", p.Target().Name(), "GetKey(ctx, s.Db, req.Get", name.Go(), "()); err != nil {")
			w.P("		return nil, err")
			w.P("	} else {")
			w.P("		q.Set", name.Ent(), "ID(k)")
			w.P("	}")
		default:
			panic("unknown type of graph prop")
		}
		if p.HasDefault() {
			w.P("	} else {")
			w.Pf("		q.Set%s(", name.Ent())
			switch p_ := p.(type) {
			case graph.Field:
				t := p_.Type()
				switch t {
				case ormpb.Type_TYPE_STRING:
					w.Pf("%q", "")
				case ormpb.Type_TYPE_BYTES:
					w.Pf("[]byte{}")
				case ormpb.Type_TYPE_ENUM:
					w.Pf("0")
				case ormpb.Type_TYPE_UUID:
					w.Pf("%s()", work.PkgUuid.Ident("New"))
				case ormpb.Type_TYPE_TIME:
					w.Pf("%s().UTC()", work.PkgTime.Ident("Now"))
				default:
					switch t.Decay() {
					case ormpb.Type_TYPE_FLOAT,
						ormpb.Type_TYPE_INT,
						ormpb.Type_TYPE_UINT:
						w.Pf("0")
					case ormpb.Type_TYPE_BOOL:
						w.Pf("false")
					case ormpb.Type_TYPE_MESSAGE:
						w.Pf("nil")
					default:
						panic(fmt.Errorf("default value for type %s is not implemented", t))
					}
				}

			case graph.Edge:
				panic("default value for edge is not implemented")
			default:
				panic("unknown type of graph prop")
			}
			w.P(")")
		}

		if p.IsOptional() {
			w.P("	}")
		}
	}
	w.P("")
	w.P("v, err := q.Save(ctx)")
	w.P("if err != nil {")
	w.P("	return nil, err")
	w.P("}")
	w.P("")
	w.P("return v.Proto(), nil")
	w.P("}")
	w.P("")
}
