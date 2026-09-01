package main

import (
	"flag"

	"google.golang.org/protobuf/compiler/protogen"
)

func main() {
	h := Handler{}

	var flags flag.FlagSet
	flags.StringVar(&h.Schema.Namer, "schema.namer", "schema/{{ .Name }}.go", "golang text template for output filename of Ent schema")
	flags.StringVar(&h.Ent.Namer, "ent.namer", "ent/{{ .Name }}.g.go", "golang text template for output filename of utility code for Ent generated code")
	flags.StringVar(&h.Server.Namer, "server.namer", "server/bare/{{ .Name }}.g.go", "golang text template for output filename of service server implementation")
	flags.StringVar(&h.Store.Name, "store.name", "server/bare/store.g.go", "output filename for store server implementation")

	opts := protogen.Options{ParamFunc: flags.Set}
	opts.Run(h.Run)
}
