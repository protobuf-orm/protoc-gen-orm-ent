package main

import (
	"strings"
	"testing"

	"github.com/protobuf-orm/protobuf-orm/ormpb"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	d "google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

func handler() *Handler {
	return &Handler{
		Schema: SchemaOpts{Namer: "schema/{{ .Name }}.go"},
		Ent:    EntOpts{Namer: "ent/{{ .Name }}.g.go"},
		Server: ServerOpts{Namer: "server/bare/{{ .Name }}.g.go"},
		Store:  StoreOpts{Name: "server/bare/store.g.go"},
	}
}

// request is one file with one entity, Counter, whose verbs are `rpc`.
func request(t *testing.T, rpc *ormpb.RpcOptions) *pluginpb.CodeGeneratorRequest {
	t.Helper()
	msg := &d.MessageOptions{}
	proto.SetExtension(msg, ormpb.E_Message, ormpb.MessageOptions_builder{Rpc: rpc}.Build())
	key := &d.FieldOptions{}
	proto.SetExtension(key, ormpb.E_Field, ormpb.FieldOptions_builder{Key: proto.Bool(true)}.Build())

	f := &d.FileDescriptorProto{
		Name:    proto.String("t/counter.proto"),
		Package: proto.String("t"),
		Syntax:  proto.String("proto3"),
		Options: &d.FileOptions{GoPackage: proto.String("example.com/t")},
		MessageType: []*d.DescriptorProto{{
			Name:    proto.String("Counter"),
			Options: msg,
			Field: []*d.FieldDescriptorProto{
				{Name: proto.String("id"), Number: proto.Int32(1), Type: d.FieldDescriptorProto_TYPE_STRING.Enum(), Options: key},
				{Name: proto.String("count"), Number: proto.Int32(2), Type: d.FieldDescriptorProto_TYPE_INT64.Enum()},
			},
		}},
	}
	return &pluginpb.CodeGeneratorRequest{
		FileToGenerate: []string{f.GetName()},
		ProtoFile:      []*d.FileDescriptorProto{f},
	}
}

func run(t *testing.T, rpc *ormpb.RpcOptions) error {
	t.Helper()
	p, err := protogen.Options{}.New(request(t, rpc))
	if err != nil {
		t.Fatal(err)
	}
	return handler().Run(p)
}

// A Patch or an Apply reads the row it wrote back through the entity's Get,
// so an entity with either and no Get is refused, by name, rather than given
// a server whose write answers Unimplemented.
func TestAWriteWithoutGetIsRefused(t *testing.T) {
	for _, c := range []struct {
		name string
		rpc  *ormpb.RpcOptions
		want string
	}{
		{
			"patch",
			ormpb.RpcOptions_builder{Add: &ormpb.RpcAdd{}, Patch: &ormpb.RpcPatch{}}.Build(),
			"t.Counter has Patch and no Get, and protoc-gen-orm-ent needs a Get for it: a Patch reads the row it wrote back through Get",
		},
		{
			"apply",
			ormpb.RpcOptions_builder{Apply: &ormpb.RpcApply{}, Erase: &ormpb.RpcErase{}}.Build(),
			"t.Counter has Apply and no Get, and protoc-gen-orm-ent needs a Get for it: an Apply reads the row it wrote back through Get",
		},
		{
			"both",
			ormpb.RpcOptions_builder{Patch: &ormpb.RpcPatch{}, Apply: &ormpb.RpcApply{}}.Build(),
			"t.Counter has Patch and Apply and no Get, and protoc-gen-orm-ent needs a Get for them: a Patch or an Apply reads the row it wrote back through Get",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := run(t, c.rpc)
			if err == nil {
				t.Fatal("generated a server for a write with no Get")
			}
			if got := err.Error(); !strings.HasSuffix(got, c.want) {
				t.Errorf("got  %q\nwant it to end %q", got, c.want)
			}
		})
	}
}

// Add and Erase read nothing back through Get, and are not refused.
func TestAddAndEraseWithoutGetAreNot(t *testing.T) {
	if err := run(t, ormpb.RpcOptions_builder{Add: &ormpb.RpcAdd{}, Erase: &ormpb.RpcErase{}}.Build()); err != nil {
		t.Fatal(err)
	}
}
