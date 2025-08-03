package main

import (
	"flag"

	"google.golang.org/protobuf/compiler/protogen"
)

func main() {
	h := Handler{}

	var flags flag.FlagSet
	flags.StringVar(&h.NamerSchema, "namer-schema", "schema/{{ .Name }}.go", "golang text template for output filename of Ent schema")
	flags.StringVar(&h.NamerEnt, "namer-ent", "ent/{{ .Name }}.g.go", "golang text template for output filename of utility code for Ent generated code")
	flags.StringVar(&h.NamerServer, "namer-server", "server/bare/{{ .Name }}.g.go", "golang text template for output filename of service server implementation")

	opts := protogen.Options{ParamFunc: flags.Set}
	opts.Run(h.Run)
}
