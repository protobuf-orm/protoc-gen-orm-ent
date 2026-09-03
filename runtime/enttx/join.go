package enttx

import (
	"context"

	"github.com/protobuf-orm/ent"
)

// Txn is [ent.Txn].
//
// Deprecated: use ent's.
type Txn[C any] = ent.Txn[C]

// Db is [ent.Db].
//
// Deprecated: use ent's.
type Db[C any, T Txn[C]] = ent.Db[C, T]

// Joined is [ent.Joined].
//
// Deprecated: use ent's.
type Joined[C any] = ent.Joined[C]

// Join is [ent.JoinTx].
//
// Deprecated: call ent's directly.
func Join[C Db[C, T], T Txn[C]](ctx context.Context, db C, want bool) (Joined[C], error) {
	return ent.JoinTx[C, T](ctx, db, want)
}
