package bare_test

import (
	"crypto/rand"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/lesomnus/protobuf-patch/patch"
	"github.com/lesomnus/z"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/ent"
	"github.com/protobuf-orm/protoc-gen-orm-ent/internal/apptest/server/bare"
)

// A repeated field is kept as a JSON array of the proto's own element type,
// whatever its `type` says. So saying TYPE_UUID on a `repeated bytes` writes
// the schema line the field without it has, and changes nothing stored.
func TestRepeatedTypeIsTheProtoElementType(t *testing.T) {
	b, err := os.ReadFile("../../schema/list.go")
	require.NoError(t, err)
	line := func(name string) string {
		m := regexp.MustCompile(`field\.Json\("` + name + `", ([^)]*)\)`).FindStringSubmatch(string(b))
		require.NotNil(t, m, "no line for %s", name)
		return m[1]
	}
	require.Equal(t, "[][]byte{}", line("blobs"))
	require.Equal(t, line("blobs"), line("subjects"), "TYPE_UUID changed how the elements are kept")
	require.True(t, strings.HasPrefix(line("times"), "[]*timestamppb.Timestamp{}"), line("times"))
}

func uuidBytes() []byte {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return b
}

// A row with two identifiers and two times is added, read back equal through
// Get, patched and read back, and has an element appended by Apply.
func TestRepeatedTypedFieldsRoundTrip(t *testing.T) {
	stores(t, func(t *testing.T, db *ent.Client) {
		x := require.New(t)
		ctx := t.Context()
		s, err := bare.NewServer(db)
		x.NoError(err)

		at := time.Date(2026, 11, 25, 9, 30, 0, 123456000, time.UTC)
		u1, u2, u3 := uuidBytes(), uuidBytes(), uuidBytes()
		id := unique("list")
		v, err := s.ListField().Add(ctx, pb.ListFieldAddRequest_builder{
			Id:       z.Ptr(id),
			Subjects: [][]byte{u1, u2},
			Blobs:    [][]byte{[]byte("a")},
			Times:    []*timestamppb.Timestamp{timestamppb.New(at), timestamppb.New(at.Add(time.Hour))},
		}.Build())
		x.NoError(err)
		t.Cleanup(func() { _, _ = s.ListField().Erase(t.Context(), v.Ref()) })
		x.Equal([][]byte{u1, u2}, v.GetSubjects())

		get := func() *pb.ListField {
			got, err := s.ListField().Get(ctx, v.Ref().Pick().WithSelect(func(s *pb.ListFieldSelect) { s.SetAll(true) }))
			x.NoError(err)
			return got
		}
		got := get()
		x.Equal([][]byte{u1, u2}, got.GetSubjects())
		x.Equal([][]byte{[]byte("a")}, got.GetBlobs())
		x.Len(got.GetTimes(), 2)
		x.True(proto.Equal(timestamppb.New(at), got.GetTimes()[0]))
		x.True(proto.Equal(timestamppb.New(at.Add(time.Hour)), got.GetTimes()[1]))

		_, err = s.ListField().Patch(ctx, pb.ListFieldPatchRequest_builder{
			Ref:      v.Ref(),
			Subjects: [][]byte{u3},
			Times:    []*timestamppb.Timestamp{timestamppb.New(at.Add(2 * time.Hour))},
		}.Build())
		x.NoError(err)
		got = get()
		x.Equal([][]byte{u3}, got.GetSubjects())
		x.Equal([][]byte{[]byte("a")}, got.GetBlobs(), "a field the patch did not name")
		x.Len(got.GetTimes(), 1)
		x.True(proto.Equal(timestamppb.New(at.Add(2*time.Hour)), got.GetTimes()[0]))

		doc, err := patch.New("apptest.ListField",
			patch.Target(patch.Append()).In(patch.Name("subjects")).Insert(patch.Bytes(u1)),
		)
		x.NoError(err)
		_, err = s.ListField().Apply(ctx, pb.ListFieldApplyRequest_builder{Ref: v.Ref(), Patch: doc}.Build())
		x.NoError(err)
		x.Equal([][]byte{u3, u1}, get().GetSubjects())
	})
}
