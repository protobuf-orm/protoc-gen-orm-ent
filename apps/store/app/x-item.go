package app

import "github.com/protobuf-orm/protoc-gen-orm-ent/internal/work"

// xItemError emits ItemError, which says that a refusal was about one item of
// a call that carries many, and which.
//
// A call of a thousand rows refused with "tenant: not found" leaves its caller
// to find the row by trying them one at a time. So the refusal names the item
// twice: in the message, for a person, and in the field path of the
// google.rpc.BadRequest it carries, for a program -- `items[3].tenant`, the
// path that detail documents for a repeated field, prefixed onto whatever path
// the refusal already had.
//
// The code is the refusal's own. Only an InvalidArgument is the request's fault
// in a way BadRequest describes, so only one of those gains a field violation
// it did not have; a NotFound or a PermissionDenied keeps its details as they
// were and says the item in its message.
func (w *Work) xItemError() {
	w.P("// ItemError is `err`, about the item at `i` of a call that carries many --")
	w.P("// an AddMany -- said so that its caller can tell which: in the code `err`")
	w.P("// had, after `items[i]: ` in the message, and, for an InvalidArgument, with")
	w.P("// every field of its google.rpc.BadRequest under `items[i]` -- or one that")
	w.P("// names the item, when it had none. A layer that refuses one item of many")
	w.P("// answers with this, as the generated AddMany does.")
	w.P("func ItemError(i int, err error) error {")
	w.P("	if err == nil {")
	w.P("		return nil")
	w.P("	}")
	w.P("")
	w.P("	at := ", work.PkgFmt.Ident("Sprintf"), "(\"items[%d]\", i)")
	w.P("	st := ", work.PkgGrpcStatus.Ident("Convert"), "(err)")
	w.P("	out := ", work.PkgGrpcStatus.Ident("New"), "(st.Code(), at+\": \"+st.Message())")
	w.P("")
	w.P("	details := []", work.PkgProtoAdapt.Ident("MessageV1"), "{}")
	w.P("	named := false")
	w.P("	for _, d := range st.Details() {")
	w.P("		if b, ok := d.(*", work.PkgErrDetails.Ident("BadRequest"), "); ok {")
	w.P("			vs := make([]*", work.PkgErrDetails.Ident("BadRequest_FieldViolation"), ", len(b.GetFieldViolations()))")
	w.P("			for k, v := range b.GetFieldViolations() {")
	w.P("				field := at")
	w.P("				switch f := v.GetField(); {")
	w.P("				case f == \"\":")
	w.P("				case f[0] == '[':")
	w.P("					field += f")
	w.P("				default:")
	w.P("					field += \".\" + f")
	w.P("				}")
	w.P("				vs[k] = &", work.PkgErrDetails.Ident("BadRequest_FieldViolation"), "{Field: field, Description: v.GetDescription()}")
	w.P("			}")
	w.P("			details = append(details, &", work.PkgErrDetails.Ident("BadRequest"), "{FieldViolations: vs})")
	w.P("			named = true")
	w.P("			continue")
	w.P("		}")
	w.P("		if m, ok := d.(", work.PkgProtoAdapt.Ident("MessageV1"), "); ok {")
	w.P("			details = append(details, m)")
	w.P("		}")
	w.P("	}")
	w.P("	if !named && st.Code() == ", work.PkgGrpcCodes.Ident("InvalidArgument"), " {")
	w.P("		details = append(details, &", work.PkgErrDetails.Ident("BadRequest"), "{")
	w.P("			FieldViolations: []*", work.PkgErrDetails.Ident("BadRequest_FieldViolation"), "{{Field: at, Description: st.Message()}},")
	w.P("		})")
	w.P("	}")
	w.P("	if len(details) == 0 {")
	w.P("		return out.Err()")
	w.P("	}")
	w.P("")
	w.P("	with, err := out.WithDetails(details...)")
	w.P("	if err != nil {")
	w.P("		return out.Err()")
	w.P("	}")
	w.P("	return with.Err()")
	w.P("}")
	w.P("")
}
