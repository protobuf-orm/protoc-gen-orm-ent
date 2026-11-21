package app

import (
	"strconv"

	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"
	"google.golang.org/protobuf/compiler/protogen"
)

// Admission is below application layers: every generated write reaches it,
// and a guarded ent mutation must be accounted for before its transaction can
// commit. A layer alone cannot establish either property.
func (w *Work) xAdmission() {
	stringsPkg := protogen.GoImportPath("strings")
	w.P("// Admission may refuse a write after its SQL checks and before its transaction commits.")
	w.P("// Change includes the request and locked previous image. These are raw protobuf")
	w.P("// messages; an adapter must redact secrets before passing them to external policy.")
	w.P("// Each call owns its messages; changing them cannot change the write or its record.")
	w.P("// It receives no server to write through. A refusal preserves its status code.")
	w.P("type Admission interface { Admit(", work.IdentContext, ", Change) error }")
	w.P("type admission struct { check Admission; entities map[string]bool; types map[string]string; methods map[string]string }")
	w.P("type admissionPending struct { count map[string]int; sealed bool }")
	w.P("type admissionOrigin struct { method string; request ", work.PkgProto.Ident("Message"), " }")
	w.P("type admissionOriginKey struct{}")
	w.P("type admissionTrustedKey struct{}")
	// Admission needs a completion ledger even for a write that owns its
	// transaction. ent.JoinTx ordinarily starts ent's own transaction, which
	// carries no Keep. BeginTx starts one whose commit can check our ledger.
	// Joined still owns partial-write refusal when joining an app transaction.
	w.P("type writeTx struct { ", work.PkgEnt.Ident("Joined"), "[*", w.Ent.Ident("Client"), "]; end ", "interface{Commit()error;Rollback()error}", " }")
	w.P("func(t writeTx) Commit()error {if err:=t.Joined.Commit();err!=nil{return err};if t.end!=nil{return t.end.Commit()};return nil}")
	w.P("func(t writeTx) Close(){t.Joined.Close();if t.end!=nil{_ = t.end.Rollback()}}")
	w.P("func join(ctx ", work.IdentContext, ", db *", w.Ent.Ident("Client"), ", want, guarded bool)(writeTx,error){")
	w.P("var end ", "interface{Commit()error;Rollback()error}")
	w.P("if guarded {drv,tx,err:=", work.PkgEntDialect.Ident("BeginTx"), "(ctx,db.Driver());if err!=nil{return writeTx{},err};end=tx;db=db.WithDriver(drv)}")
	w.P("j,err:=", work.PkgEnt.Ident("JoinTx"), "[*", w.Ent.Ident("Client"), ",*", w.Ent.Ident("Tx"), "](ctx,db,want);if err!=nil {if end!=nil{_ = end.Rollback()};return writeTx{},err};return writeTx{j,end},nil }")
	w.P("// Origin carries the outer RPC through writes it makes in-process. It copies the")
	w.P("// message so normalization or a caller reusing a request cannot rewrite the ask.")
	w.P("func Origin(ctx ", work.IdentContext, ", method string, req ", work.PkgProto.Ident("Message"), ") ", work.IdentContext, " { return ", work.PkgContext.Ident("WithValue"), "(ctx, admissionOriginKey{}, admissionOrigin{method, ", work.PkgProto.Ident("Clone"), "(req)}) }")
	w.P("func request(ctx ", work.IdentContext, ", req ", work.PkgProto.Ident("Message"), ") ", work.IdentContext, " { if _,ok:=ctx.Value(admissionOriginKey{}).(admissionOrigin);ok{return ctx};return Origin(ctx,\"\",req) }")
	w.P("// TrustAdmission is an explicit server-side bypass for deployment-owned work.")
	w.P("// It is never read from request metadata. The ordinary recorder still runs.")
	w.P("func TrustAdmission(ctx ", work.IdentContext, ") ", work.IdentContext, " { return ", work.PkgContext.Ident("WithValue"), "(ctx, admissionTrustedKey{}, true) }")
	w.P("func admissionTrusted(ctx ", work.IdentContext, ")bool {v,_:=ctx.Value(admissionTrustedKey{}).(bool);return v}")
	w.P("// WithAdmission protects the named protobuf entities, or every entity when none")
	w.P("// are named. Install once on the shared ent client. WithDriver retains its guard.")
	w.P("// Start shared transactions with dialect.BeginTx; an ent.Tx has no commit ledger.")
	w.P("// Direct ent writes need a kept transaction and a matching generated admission;")
	w.P("// a missing admission refuses commit even when an application ignores an error.")
	w.P("// A newly constructed client or raw SQL is outside this guard.")
	w.P("func WithAdmission(check Admission, entities ...string) Option { return func(s *Server) error {")
	w.P("if s.admission!=nil {return ", work.PkgFmt.Ident("Errorf"), "(\"admission: %w\",ErrTwice)}")
	w.P("if check==nil {return ", work.PkgFmt.Ident("Errorf"), "(\"admission: no decision maker\")}")
	w.P("a:=&admission{check:check,entities:map[string]bool{},types:map[string]string{},methods:map[string]string{}}")
	for _, e := range w.Entities {
		full := string(e.FullName())
		w.P("a.types[", strconv.Quote(e.Name()), "] = ", strconv.Quote(full))
		w.P("a.methods[", strconv.Quote("/"+full+"Service"), "] = ", strconv.Quote(full))
	}
	w.P("for _,name:=range entities {found:=false;for _,v:=range a.types {if v==name {found=true;break}};if !found{return ", work.PkgFmt.Ident("Errorf"), "(\"admission: unknown entity %q\",name)};a.entities[name]=true}")
	w.P("if len(entities)==0 {for _,name:=range a.types {a.entities[name]=true}}")
	w.P("s.admission=a;s.Db.Use(a.guard());return nil }")
	w.P("}")
	w.P("func(s Store) images(method string) SnapshotOptions {")
	w.P("out:=snapshotOptions(s.Rec,method)")
	w.P("if a:=s.admission;a!=nil {service,_,_:=", stringsPkg.Ident("Cut"), "(", stringsPkg.Ident("TrimPrefix"), "(method,\"/\"),\"/\");if a.entities[a.methods[\"/\"+service]] {out.Previous=true;out.Value=true}}")
	w.P("return out }")
	w.P("func(a *admission) pending(k *", work.PkgEntDialect.Ident("Keep"), ") *admissionPending {")
	w.P("return k.Value(a,func() any {p:=&admissionPending{count:map[string]int{}};k.BeforeCommit(func(", work.IdentContext, ")error {p.sealed=true;for name,n:=range p.count {if n!=0{return ", work.PkgGrpcStatus.Ident("Errorf"), "(", work.PkgGrpcCodes.Ident("FailedPrecondition"), ",\"%s: an ent write bypassed admission\",name)}};return nil});return p}).(*admissionPending) }")
	w.P("func(a *admission) guard() ", w.Ent.Ident("Hook"), " { return func(next ", w.Ent.Ident("Mutator"), ") ", w.Ent.Ident("Mutator"), " {return ", w.Ent.Ident("MutateFunc"), "(func(ctx ", work.IdentContext, ", m ", w.Ent.Ident("Mutation"), ") (", w.Ent.Ident("Value"), ",error) {")
	w.P("name:=a.types[m.Type()];if !a.entities[name]||admissionTrusted(ctx){return next.Mutate(ctx,m)}")
	w.P("client,ok:=m.(interface{Client()*", w.Ent.Ident("Client"), "});if !ok {return nil,", work.PkgGrpcStatus.Ident("Error"), "(", work.PkgGrpcCodes.Ident("Internal"), ",\"admission: mutation has no client\")}")
	w.P("k,ok:=", work.PkgEntDialect.Ident("KeepOf"), "(client.Client().Driver());if !ok{return nil,", work.PkgGrpcStatus.Ident("Error"), "(", work.PkgGrpcCodes.Ident("FailedPrecondition"), ",\"admission: use the generated server or an explicit trusted write\")}")
	w.P("p:=a.pending(k);if p.sealed {err:=", work.PkgGrpcStatus.Ident("Error"), "(", work.PkgGrpcCodes.Ident("FailedPrecondition"), ",\"admission: protected writes cannot begin during commit\");k.Refuse(err);return nil,err};v,err:=next.Mutate(ctx,m);if err!=nil {k.Refuse(err);return v,err}")
	w.P("if n,ok:=v.(int);!ok||n!=0 {p.count[name]++};return v,nil")
	w.P("})} }")
	w.P("func(a *admission) admit(ctx ", work.IdentContext, ", db *", w.Ent.Ident("Client"), ", c Change) error {")
	w.P("if a==nil||!a.entities[c.Entity]||admissionTrusted(ctx){return nil}")
	w.P("k,ok:=", work.PkgEntDialect.Ident("KeepOf"), "(db.Driver());if !ok {return ", work.PkgGrpcStatus.Ident("Error"), "(", work.PkgGrpcCodes.Ident("FailedPrecondition"), ",\"admission: write has no kept transaction\")}")
	w.P("p:=a.pending(k);if p.count[c.Entity]==0 {err:=", work.PkgGrpcStatus.Ident("Error"), "(", work.PkgGrpcCodes.Ident("FailedPrecondition"), ",\"admission: mutation guard was not installed\");k.Refuse(err);return err}")
	w.P("c.Request=", work.PkgProto.Ident("Clone"), "(c.Request);c.Previous=", work.PkgProto.Ident("Clone"), "(c.Previous);c.Value=", work.PkgProto.Ident("Clone"), "(c.Value)")
	w.P("if c.Patch!=nil {c.Patch=", work.PkgProto.Ident("Clone"), "(c.Patch).(*", work.PkgPatchPb.Ident("Patch"), ")};if k,ok:=c.Key.([]byte);ok{c.Key=append([]byte(nil),k...)}")
	w.P("err:=ctx.Err();if err==nil {err=a.check.Admit(ctx,c)};if err==nil {err=ctx.Err()};if err!=nil {k.Refuse(err);return err};p.count[c.Entity]--;return nil }")
	w.P("")
}
