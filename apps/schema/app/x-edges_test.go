package app

import (
	"context"
	"testing"

	"github.com/protobuf-orm/protobuf-orm/graph"
	"github.com/protobuf-orm/protobuf-orm/ormpb"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	d "google.golang.org/protobuf/types/descriptorpb"
)

func entityOpts() *d.MessageOptions {
	o := &d.MessageOptions{}
	proto.SetExtension(o, ormpb.E_Message, &ormpb.MessageOptions{})
	return o
}

func keyField() *d.FieldDescriptorProto {
	o := &d.FieldOptions{}
	proto.SetExtension(o, ormpb.E_Field, ormpb.FieldOptions_builder{Key: proto.Bool(true)}.Build())
	return &d.FieldDescriptorProto{Name: proto.String("id"), Number: proto.Int32(1), Type: d.FieldDescriptorProto_TYPE_INT32.Enum(), Options: o}
}

func edgeField(name string, number int32, target string, list bool, opts *ormpb.EdgeOptions) *d.FieldDescriptorProto {
	o := &d.FieldOptions{}
	proto.SetExtension(o, ormpb.E_Edge, opts)
	f := &d.FieldDescriptorProto{
		Name: proto.String(name), Number: proto.Int32(number),
		Type: d.FieldDescriptorProto_TYPE_MESSAGE.Enum(), TypeName: proto.String(".t." + target),
		Options: o,
	}
	if list {
		f.Label = d.FieldDescriptorProto_LABEL_REPEATED.Enum()
	}
	return f
}

func from(name string, number int32) *ormpb.Ref {
	return ormpb.Ref_builder{Name: name, Number: number}.Build()
}

// onDeleteOf puts an edge's on_delete on the ent edge that is written `To`,
// which is where ent reads it, whichever of the pair protobuf-orm says it on.
// The shapes whose key holder is written `From` cannot be generated end to end
// here (a list edge does not reach ent's package, and a one-to-one's Ref does
// not reach a Pick), so this is where they are held.
func TestOnDeleteOf(t *testing.T) {
	f := &d.FileDescriptorProto{
		Name:    proto.String("t.proto"),
		Package: proto.String("t"),
		Syntax:  proto.String("proto3"),
		MessageType: []*d.DescriptorProto{
			// Each edge with no inverse says it itself.
			{Name: proto.String("Target"), Options: entityOpts(), Field: []*d.FieldDescriptorProto{keyField()}},
			{Name: proto.String("Row"), Options: entityOpts(), Field: []*d.FieldDescriptorProto{
				keyField(),
				edgeField("unsaid", 2, "Target", false, ormpb.EdgeOptions_builder{}.Build()),
				edgeField("restrict", 3, "Target", false, ormpb.EdgeOptions_builder{OnDelete: ormpb.OnDelete_ON_DELETE_RESTRICT.Enum()}.Build()),
				edgeField("cascade", 4, "Target", false, ormpb.EdgeOptions_builder{OnDelete: ormpb.OnDelete_ON_DELETE_CASCADE.Enum()}.Build()),
				edgeField("set_null", 5, "Target", false, ormpb.EdgeOptions_builder{Nullable: proto.Bool(true), OnDelete: ormpb.OnDelete_ON_DELETE_SET_NULL.Enum()}.Build()),
			}},
			// A many-to-one whose single side is written from its list: it
			// holds the key, and ent reads the action from the list.
			{Name: proto.String("Group"), Options: entityOpts(), Field: []*d.FieldDescriptorProto{
				keyField(),
				edgeField("members", 2, "Member", true, ormpb.EdgeOptions_builder{}.Build()),
			}},
			{Name: proto.String("Member"), Options: entityOpts(), Field: []*d.FieldDescriptorProto{
				keyField(),
				edgeField("group", 2, "Group", false, ormpb.EdgeOptions_builder{From: from("members", 2), Nullable: proto.Bool(true), OnDelete: ormpb.OnDelete_ON_DELETE_CASCADE.Enum()}.Build()),
			}},
			// A many-to-one whose list is written from its single side: the
			// single side holds the key and is the one written `To`.
			{Name: proto.String("Tree"), Options: entityOpts(), Field: []*d.FieldDescriptorProto{
				keyField(),
				edgeField("parent", 2, "Tree", false, ormpb.EdgeOptions_builder{Nullable: proto.Bool(true), OnDelete: ormpb.OnDelete_ON_DELETE_RESTRICT.Enum()}.Build()),
				edgeField("children", 3, "Tree", true, ormpb.EdgeOptions_builder{From: from("parent", 2)}.Build()),
			}},
			// A one-to-one: the side written from the other holds the key.
			{Name: proto.String("Node"), Options: entityOpts(), Field: []*d.FieldDescriptorProto{
				keyField(),
				edgeField("next", 2, "Node", false, ormpb.EdgeOptions_builder{Nullable: proto.Bool(true)}.Build()),
				edgeField("prev", 3, "Node", false, ormpb.EdgeOptions_builder{From: from("next", 2), Nullable: proto.Bool(true), OnDelete: ormpb.OnDelete_ON_DELETE_SET_NULL.Enum()}.Build()),
			}},
		},
	}
	fd, err := protodesc.NewFile(f, protoregistry.GlobalFiles)
	if err != nil {
		t.Fatal(err)
	}
	g := graph.NewGraph()
	if err := graph.Parse(context.Background(), g, fd); err != nil {
		t.Fatal(err)
	}

	edge := func(entity, name string) graph.Edge {
		e, ok := g.Entities[protoreflect.FullName("t."+entity)]
		if !ok {
			t.Fatalf("no entity %s", entity)
		}
		for v := range e.Edges() {
			if v.Name() == name {
				return v
			}
		}
		t.Fatalf("no edge %s.%s", entity, name)
		return nil
	}
	for _, c := range []struct {
		entity, edge string
		want         string
	}{
		{"Row", "unsaid", ""},
		{"Row", "restrict", "Restrict"},
		{"Row", "cascade", "Cascade"},
		{"Row", "set_null", "SetNull"},
		{"Group", "members", "Cascade"},
		{"Member", "group", ""},
		{"Tree", "parent", "Restrict"},
		{"Tree", "children", ""},
		{"Node", "next", "SetNull"},
		{"Node", "prev", ""},
	} {
		if got := onDeleteOf(edge(c.entity, c.edge)); got != c.want {
			t.Errorf("%s.%s: %q, want %q", c.entity, c.edge, got, c.want)
		}
	}
}
