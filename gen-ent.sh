# internal/apptest is its own module (it owns the ent dependency), so ent's
# code generator must run from there.
#
# The version is written out, and `go run pkg@version` rather than a tool
# directive, because ent's CLI drags in a `genproto` from 2020 that is
# ambiguous with the split-out one everything else uses -- as a dependency of
# this module it breaks the build; resolved outside it, it bothers nobody.
#
# It has to match `entgo.io/ent` in internal/apptest/go.mod: the generator and
# the runtime it generates against are one thing.
ENT=v0.14.5

cd "$(dirname "$0")/internal/apptest" && go run "entgo.io/ent/cmd/ent@${ENT}" generate ./schema --target ./ent --feature sql/modifier
