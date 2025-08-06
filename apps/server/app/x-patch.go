package app

import (
	"github.com/protobuf-orm/protobuf-orm/graph"
	"github.com/protobuf-orm/protobuf-orm/ormpb"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"
)

// func (s MissionServiceServer) Patch(ctx context.Context, req *odin.MissionPatchRequest) (*emptypb.Empty, error) {
// 	id, err := MissionGetId(ctx, s.Db, req.GetKey())
// 	if err != nil {
// 		return nil, err
// 	}

// 	q := s.Db.Mission.UpdateOneID(id)
// 	if req.HasTenant() {
// 		if id, err := TenantGetId(ctx, s.Db, req.GetTenant()); err != nil {
// 			return nil, err
// 		} else {
// 			q.SetTenantID(id)
// 		}
// 	}
// 	if req.GetSiteNull() {
// 		q.ClearSite()
// 	} else if req.HasSite() {
// 		if id, err := SiteGetId(ctx, s.Db, req.GetSite()); err != nil {
// 			return nil, err
// 		} else {
// 			q.SetSiteID(id)
// 		}
// 	}
// 	if req.HasAlias() {
// 		q.SetAlias(req.GetAlias())
// 	}

func (w *fileWork) xPatch() {
	name := w.Ident.GoName
	w.P("func (s ", name, "ServiceServer) Patch(",
		/* */ "ctx ", work.PkgContext.Ident("Context"), ",",
		/* */ "req *", w.Src.GoImportPath.Ident(name+"PatchRequest"),
		") (*", work.IdentEmpty, ", error) {")
	w.P("	p, err := ", name, "Pick(req.GetTarget())")
	w.P("	if err != nil {")
	w.P("		return nil, err")
	w.P("	}")
	w.P("")
	w.P("	q := s.Db.", name, ".Update().Where(p)")
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

		if p.IsOptional() {
			w.P("	if req.Has", name.Go(), "() {")
		}
		switch p_ := p.(type) {
		case graph.Field:
			t := p_.Type()
			switch t {
			case ormpb.Type_TYPE_UUID:
				w.P("if v, err := ", work.PkgGoogleUuid.Ident("FromBytes"), "(req.Get", name.Go(), "()); err != nil {")
				w.P("	return nil, ", work.PkgGrpcStatus.Ident("Errorf"), "(", work.PkgGrpcCodes.Ident("InvalidArgument"), ", \"", name, ": %s\", err)")
				w.P("} else {")
				w.P("	q.Set", name.Ent(), "(v)")
				w.P("}")
			case ormpb.Type_TYPE_TIME:
				w.P("q.Set", name.Ent(), "(req.Get", name.Go(), "().AsTime())")
			default:
				w.P("q.Set", name.Ent(), "(req.Get", name.Go(), "())")
			}

		case graph.Edge:
			// edges = append(edges, name)
		default:
			panic("unknown type of graph prop")
		}
		if p.IsOptional() {
			w.P("}")
		}
	}
	w.P("")
	w.P("if _, err := q.Save(ctx); err != nil {")
	w.P("	return nil, err")
	w.P("}")
	w.P("")
	w.P("return nil, nil")
	w.P("}")
	w.P("")
}
