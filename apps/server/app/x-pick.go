package app

import (
	"fmt"
	"slices"
	"strings"

	"github.com/protobuf-orm/protobuf-orm/graph"
	"github.com/protobuf-orm/protobuf-orm/ormpb"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"
	"google.golang.org/protobuf/compiler/protogen"
)

func (w *fileWork) xPick() {
	name_x := w.Ident.GoName
	pkg := w.Src.GoImportPath
	x := w.ent + "/" + protogen.GoImportPath(strings.ToLower(name_x))
	predicate := (w.ent + "/predicate").Ident(name_x)

	// A reference is composed **into** another entity's, which is why the
	// erased predicate is here as well as in <Entity>Narrow. A unique index
	// that names an edge emits `Has<Target>With(<Target>Pick(...))`, and that
	// read never reaches a Narrow of the target -- so without this, a child of
	// an erased row is found by naming its parent, and found through the wall,
	// because a wall narrows the child's tenancy path and not the parent's
	// liveness. <Entity>Id has the same shape and the same hole.
	//
	// Narrow keeps its own copy rather than deferring to this one: it is "the
	// one place a read is narrowed" for reads that name the row directly, and
	// a predicate that holds twice costs a pair of parentheses.
	del := w.Entity.GetErasedField()
	name_pick := name_x + "Pick"
	if del != nil {
		name_pick = "pick" + name_x

		w.P("// ", name_x, "Pick answers with the predicate this reference selects on,")
		w.P("// among the rows that are still here.")
		w.P("//")
		w.P("// Erasure is part of the reference and not only part of a read's scope,")
		w.P("// because a reference to a ", name_x, " is composed into the reference of")
		w.P("// whatever names one: an index over an edge asks this for a predicate and")
		w.P("// puts it inside `Has", name_x, "With`, where no narrowing of a ", name_x)
		w.P("// is ever applied. A child of an erased row would otherwise be readable by")
		w.P("// naming its parent.")
		w.P("func ", name_x, "Pick(",
			/* */ "req *", pkg.Ident(name_x), "Ref",
			") (", predicate, ", error) {")
		w.P("	p, err := ", name_pick, "(req)")
		w.P("	if err != nil {")
		w.P("		return nil, err")
		w.P("	}")
		w.P("")
		w.P("	return ", x.Ident("And"), "(", x.Ident(work.Name(del.Name()).Ent()+"IsNil"), "(), p), nil")
		w.P("}")
		w.P("")
	}

	w.P("func ", name_pick, "(",
		/* */ "req *", pkg.Ident(name_x), "Ref",
		") (", predicate, ", error) {")
	w.P("	switch req.WhichKey() {")
	for p := range w.Entity.Props() {
		if !p.IsUnique() {
			continue
		}

		name_p := work.Name(p.Name())
		switch p_ := p.(type) {
		case graph.Field:
			eq := x.Ident(name_p.Ent() + "EQ")

			w.P("	case ", pkg.Ident(name_x+"Ref_"+name_p.Go()+"_case"), ":")
			t := p_.Type()
			switch t {
			case ormpb.Type_TYPE_UUID:
				w.P("		if v, err := ", work.PkgGoogleUuid.Ident("FromBytes"), "(req.Get", name_p.Go(), "()); err != nil {")
				w.P("			return nil, ", work.PkgGrpcStatus.Ident("Errorf"), "(", work.PkgGrpcCodes.Ident("InvalidArgument"), ", \"", name_p, ": %s\", ", "err)")
				w.P("		} else {")
				w.P("			return ", eq, "(v), nil")
				w.P("		}")
			case ormpb.Type_TYPE_TIME:
				w.P("		return ", eq, "(req.Get", name_p.Go(), "().AsTime()), nil")
			default:
				w.P("		return ", eq, "(req.Get", name_p.Go(), "()), nil")
			}
		case graph.Edge:
			panic("not implemented: pick unique edge")
		default:
			panic("unknown type of graph prop")
		}
	}
	for p := range w.Entity.Indexes() {
		if !p.IsUnique() {
			continue
		}

		name_p := work.Name(p.Name())
		w.P("	case ", pkg.Ident(name_x+"Ref_"+name_p.Go()+"_case"), ":")
		w.P("		k := req.Get", name_p.Go(), "()")
		w.P("		ps := make([]", predicate, ", 0, ", len(slices.Collect(p.Props())), ")")
		for p := range p.Props() {
			name := work.Name(p.Name())
			eq := x.Ident(name.Ent() + "EQ")
			switch p_ := p.(type) {
			case graph.Field:
				t := p_.Type()
				switch t {
				case ormpb.Type_TYPE_UUID:
					w.P("		if v, err := ", work.PkgGoogleUuid.Ident("FromBytes"), "(k.Get", name.Go(), "()); err != nil {")
					w.P("			return nil, ", work.PkgGrpcStatus.Ident("Errorf"), "(", work.PkgGrpcCodes.Ident("InvalidArgument"), ", \"", name_p, ".", name, ": %s\", ", "err)")
					w.P("		} else {")
					w.P("			ps = append(ps, ", eq, "(v))")
					w.P("		}")
				case ormpb.Type_TYPE_TIME:
					w.P("		return ", eq, "(k.Get", name.Go(), "().AsTime()), nil")
				default:
					w.P("		ps = append(ps, ", eq, "(k.Get", name.Go(), "()))")
				}
			case graph.Edge:
				name_target := work.Name(p_.Target().Name())
				w.P("		if p, err := ", name_target, "Pick(k.Get", name_target.Go(), "()); err != nil {")
				w.P("			return nil, ", work.PkgGrpcStatus.Ident("Errorf"), "(", work.PkgGrpcCodes.Ident("InvalidArgument"), ", \"", name_p, ".", name, ": %s\", ", "err)")
				w.P("		} else {")
				w.P("			ps = append(ps, ", x.Ident("Has"+name_target.Go()+"With"), "(p))")
				w.P("		}")
			default:
				panic("unknown type of graph prop")
			}
		}
		w.P("		return ", x.Ident("And"), "(ps...) , nil")
	}
	w.P("	case ", pkg.Ident(fmt.Sprintf("%sRef_Key_not_set_case", name_x)), ":")
	w.P("		return nil, ", work.PkgGrpcStatus.Ident("Errorf"), "(", work.PkgGrpcCodes.Ident("InvalidArgument"), ", \"key not set: ", name_x, "\")")
	w.P("	default:")
	w.P("		return nil, ", work.PkgGrpcStatus.Ident("Errorf"), "(", work.PkgGrpcCodes.Ident("Unimplemented"), ", \"unknown type of key: %s\", req.WhichKey())")
	w.P("	}")
	w.P("}")
	w.P("")
}
