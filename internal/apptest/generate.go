// apptest is its own module because it owns the ent dependency, so ent's code
// generator runs from here rather than from the repository root.
//
// `go tool` and not `go run pkg@version`: the version then lives in go.mod
// alone. What the generator is and what the generated code is compiled against
// are one thing, and a version written out a second time is one that can
// disagree with the first -- which it did, for two commits, silently, because
// the old generator writes what is already committed.
//
// It was `go run` for a reason that has since gone: ent's CLI used to drag in
// the 2020 `google.golang.org/genproto`, which is ambiguous with the split-out
// one everything else here uses, and resolving the CLI outside this module's
// graph kept it out. Dropping the Gremlin driver took `go.opencensus.io` with
// it and that genproto with that, so there is nothing left to keep out.
package apptest

//go:generate go tool ent generate --target ./ent --feature sql/modifier ./schema
