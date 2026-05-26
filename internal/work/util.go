package work

import (
	"github.com/protobuf-orm/protobuf-orm/graph"
	"github.com/protobuf-orm/protobuf-orm/ormpb"
)

func IsDefaultIdField(p graph.Prop) bool {
	return p.Entity().Key() == p && p.Name() == "id" && p.Type() == ormpb.Type_TYPE_INT64
}
