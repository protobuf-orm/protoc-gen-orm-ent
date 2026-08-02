package entpatch

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/protobuf-orm/protobuf-orm/graph"
	"github.com/protobuf-orm/protobuf-orm/ormpb"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// arg renders a plan value as a statement argument.
//
// inner says the value lands inside a JSON document -- one entry of a map, one
// element of a list -- in which case it is marshalled rather than bound
// natively, because the surrounding expression is JSON.
//
// The conversions mirror what the ent schema generator chose for each ormpb
// type, which is why they are keyed on the prop's Type rather than on the
// protobuf kind: a UUID is bytes on the wire and a uuid.UUID in the column, and
// a version field is a Timestamp on the wire and a time.Time in the column.
func arg(p graph.Prop, v protoreflect.Value, inner bool) (any, error) {
	if inner {
		return jsonArg(p, v)
	}

	if graph.IsCollection(p) || p.Type() == ormpb.Type_TYPE_JSON {
		return jsonArg(p, v)
	}
	return scalarArg(p, v)
}

// scalarArg converts one value of the prop's own type.
func scalarArg(p graph.Prop, v protoreflect.Value) (any, error) {
	switch p.Type() {
	case ormpb.Type_TYPE_UUID:
		b, ok := v.Interface().([]byte)
		if !ok {
			return nil, fmt.Errorf("entpatch: %s is a UUID but the value is %T", p.Name(), v.Interface())
		}
		u, err := uuid.FromBytes(b)
		if err != nil {
			return nil, fmt.Errorf("entpatch: %s: %w", p.Name(), err)
		}
		return u, nil

	case ormpb.Type_TYPE_TIME:
		m, ok := v.Interface().(protoreflect.Message)
		if !ok {
			return nil, fmt.Errorf("entpatch: %s is a time but the value is %T", p.Name(), v.Interface())
		}
		ts, ok := m.Interface().(*timestamppb.Timestamp)
		if !ok {
			// A dynamic message carries the same fields; read them directly.
			d := m.Descriptor()
			secs := m.Get(d.Fields().ByName("seconds")).Int()
			nanos := m.Get(d.Fields().ByName("nanos")).Int()
			return time.Unix(secs, nanos).UTC(), nil
		}
		return ts.AsTime().UTC(), nil

	case ormpb.Type_TYPE_ENUM:
		// The schema stores an enum as int32; protobuf gives its number.
		return int32(v.Enum()), nil
	}

	return v.Interface(), nil
}

// jsonArg renders a value as the JSON text a JSON column or a JSON expression
// expects.
func jsonArg(p graph.Prop, v protoreflect.Value) (any, error) {
	switch x := v.Interface().(type) {
	case protoreflect.Map:
		out := map[string]any{}
		var err error
		x.Range(func(k protoreflect.MapKey, e protoreflect.Value) bool {
			var ev any
			ev, err = jsonScalar(e)
			if err != nil {
				return false
			}
			out[fmt.Sprint(k.Interface())] = ev
			return true
		})
		if err != nil {
			return nil, err
		}
		return marshal(out)

	case protoreflect.List:
		out := make([]any, 0, x.Len())
		for i := range x.Len() {
			ev, err := jsonScalar(x.Get(i))
			if err != nil {
				return nil, err
			}
			out = append(out, ev)
		}
		return marshal(out)
	}

	e, err := jsonScalar(v)
	if err != nil {
		return nil, err
	}
	return marshal(e)
}

func jsonScalar(v protoreflect.Value) (any, error) {
	switch x := v.Interface().(type) {
	case protoreflect.Message:
		// A message inside a JSON column is stored as its protojson form, the
		// same shape the ent conversion helpers write.
		b, err := protojson.Marshal(x.Interface())
		if err != nil {
			return nil, fmt.Errorf("entpatch: %w", err)
		}
		return json.RawMessage(b), nil
	case protoreflect.EnumNumber:
		return int32(x), nil
	case []byte:
		return x, nil
	default:
		return x, nil
	}
}

func marshal(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("entpatch: %w", err)
	}
	return string(b), nil
}

// insideArg renders a value for COMPARISON against something already inside a
// JSON document.
//
// It is not jsonArg: the surrounding predicate extracts the element first, so
// the comparison is against the element's own type. Binding the marshalled form
// would compare a quoted string to an unquoted one and never match.
func insideArg(v protoreflect.Value) (any, error) {
	switch x := v.Interface().(type) {
	case protoreflect.Message:
		b, err := protojson.Marshal(x.Interface())
		if err != nil {
			return nil, fmt.Errorf("entpatch: %w", err)
		}
		return string(b), nil
	case protoreflect.EnumNumber:
		return int32(x), nil
	case []byte:
		return string(x), nil
	default:
		return x, nil
	}
}
