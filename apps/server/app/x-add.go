package app

import (
	"fmt"
	"slices"
	"strconv"

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

	// The row and what a recorder writes about it are one write, so they are
	// done inside one transaction -- and only when there is a recorder, since
	// the row alone is a single statement with nothing to hold it together
	// with. See xJoin.
	w.xJoin("s.Rec != nil")

	edges := slices.Collect(w.Entity.Edges())
	if len(edges) > 0 {
		w.P("	ds := make([]func(v *", w.Ident, "), 0, ", len(edges), ")")
	}
	w.P("	q := st.Db.", name, ".Create()")
	for p := range w.Entity.Props() {
		// A row is added alive, so there is nothing to set: null is alive. The
		// request has no field for it either -- protoc-gen-orm-service leaves
		// it out of an AddRequest -- so this skip is what makes the emitted
		// code compile as much as it is a rule.
		//
		// It is here rather than in the switch below, where the version field
		// is handled, because the erased field is nullable and so optional: by
		// the time the switch runs, the `if req.HasX() {` that wraps an
		// optional prop has already been written and a `continue` past it would
		// leave the brace open. The version field is never optional, so the one
		// down there is safe.
		if f, ok := p.(graph.Field); ok && f.IsErased() {
			continue
		}

		// The key of a uuid-keyed entity is the one prop an app is given a say
		// in, since it is the one the server would otherwise make up out of
		// nothing. See the Minter hook in apps/store.
		//
		// Only when the schema said the server may make one up. A key declared
		// without a default is one the request has to carry, and handing that
		// case to a minter would turn a missing key into a made-up one.
		if f, ok := p.(graph.Field); ok && p.HasDefault() &&
			f.Type() == ormpb.Type_TYPE_UUID && p.Name() == w.Entity.Key().Name() {
			w.xAddKey(f)
			continue
		}

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
			if p.IsVersion() {
				switch t {
				case ormpb.Type_TYPE_TIME:
					set("st.now()")

				default:
					panic("version with type other than time not supported yet")
				}
				continue
			}

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
			m := p.Target()
			k := "k"
			switch m.Key().Type() {
			case ormpb.Type_TYPE_UUID:
				k += "[:]"
			}

			w.P("	if k, err := ", m.Name(), "GetKey(ctx, st.Db, req.Get", name.Go(), "()); err != nil {")
			w.P("		return nil, err")
			w.P("	} else {")
			w.P("		q.Set", name.Ent(), "ID(k)")
			w.P("		ds = append(ds, func(v *", w.Ident, "){")
			w.P("			v.Set", name.Ent(), "(", w.Src.GoImportPath.Ident(m.Name()+"_builder"), "{", work.Name(m.Key().Name()).Go(), ": ", k, "}.Build())")
			w.P("		})")
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
					w.Pf("st.now()")
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
	w.P("	u, err := q.Save(ctx)")
	w.P("	if err != nil {")
	w.P("		if err, ok := err.(*", w.ent.Ident("ConstraintError"), "); ok {")
	w.P("			if ", work.PkgEntSqlGraph.Ident("IsUniqueConstraintError"), "(err) {")
	w.P("				return nil, ", work.PkgGrpcStatus.Ident("Errorf"), "(", work.PkgGrpcCodes.Ident("AlreadyExists"), ", \"", name, " already exists: %s\", err.Unwrap())")
	w.P("			}")
	// A foreign-key violation here means a referenced edge target does not
	// exist; report it as NotFound, consistent with resolving an edge ref by a
	// unique index (which queries and returns NotFound).
	w.P("			if ", work.PkgEntSqlGraph.Ident("IsForeignKeyConstraintError"), "(err) {")
	w.P("				return nil, ", work.PkgGrpcStatus.Ident("Errorf"), "(", work.PkgGrpcCodes.Ident("NotFound"), ", \"", name, ": referenced entity not found: %s\", err.Unwrap())")
	w.P("			}")
	w.P("		}")
	w.P("		return nil, err")
	w.P("	}")
	w.P("")

	// The key is taken from the row rather than from the request: a request
	// may leave it out, and then the one that exists is the one the database
	// or the branch above settled on.
	w.P("	if err := record(ctx, s.Rec, st.Db, Change{")
	w.P("		By: ", w.Src.GoImportPath.Ident(name+"Service_Add_FullMethodName"), ",")
	w.P("		Key: u.ID,")
	w.P("	}); err != nil {")
	w.P("		return nil, err")
	w.P("	}")
	w.P("	if err := tx.Commit(); err != nil {")
	w.P("		return nil, err")
	w.P("	}")
	w.P("")

	if len(edges) > 0 {
		w.P("	v := u.Proto()")
		w.P("	for _, d := range ds {")
		w.P("		d(v)")
		w.P("	}")
		w.P("	return v, nil")
	} else {
		w.P("	return u.Proto(), nil")
	}
	w.P("}")
	w.P("")
}

// xAddKey emits how the key of a uuid-keyed entity is decided, which is the one
// value in an Add that the server would otherwise invent with nobody able to
// say otherwise.
//
// What the request named, if it named one, is read first and only then handed
// over. So a minter is asked about a key that is already sixteen bytes -- the
// "these are not sixteen bytes" refusal stays here, where it always was, and a
// minter is left with the question that is actually its own: whether this key
// is one this app should store a row of this kind under.
func (w *fileWork) xAddKey(f graph.Field) {
	name := work.Name(f.Name())
	has := "req.Has" + name.Go() + "()"

	w.P("	var k ", work.IdentUuid)
	w.P("	if ", has, " {")
	w.P("		if v, err := ", work.PkgUuid.Ident("FromBytes"), "(req.Get", name.Go(), "()); err != nil {")
	w.P("			return nil, ", work.PkgGrpcStatus.Ident("Errorf"), "(", work.PkgGrpcCodes.Ident("InvalidArgument"), ", \"", f.Name(), ": %s\", err)")
	w.P("		} else {")
	w.P("			k = v")
	w.P("		}")
	w.P("	}")
	w.P("	if v, err := mint(ctx, s.Mint, ", strconv.Quote(string(w.Entity.FullName())), ", k, ", has, "); err != nil {")
	w.P("		return nil, err")
	w.P("	} else {")
	w.P("		q.Set", name.Ent(), "(v)")
	w.P("	}")
}
