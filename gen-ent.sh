# internal/apptest is its own module (it owns the ent dependency), so ent's
# code generator must run from there.
#
# The version is written out, and `go run pkg@version` rather than a tool
# directive, because ent's CLI drags in a `genproto` from 2020 that is
# ambiguous with the split-out one everything else uses -- as a dependency of
# this module it breaks the build; resolved outside it, it bothers nobody.
#
# It has to match `github.com/protobuf-orm/ent` in internal/apptest/go.mod: the generator and
# the runtime it generates against are one thing.
ENT=v0.0.0-20260902014421-84763acd732b

cd "$(dirname "$0")/internal/apptest" && go run "github.com/protobuf-orm/ent/cmd/ent@${ENT}" generate --target ./ent --feature sql/modifier ./schema
