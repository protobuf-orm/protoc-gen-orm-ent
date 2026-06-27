# internal/apptest is its own module (it owns the ent dependency), so ent's
# code generator must run from there.
cd "$(dirname "$0")/internal/apptest" && go run entgo.io/ent/cmd/ent generate ./schema --target ./ent
