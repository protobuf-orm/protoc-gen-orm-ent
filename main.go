package main

import (
	"flag"

	"google.golang.org/protobuf/compiler/protogen"
)

func main() {
	h := Handler{}

	var flags flag.FlagSet
	flags.StringVar(&h.Namer, "namer", "schema/{{ .Name }}.go", "golang text template for output filename")

	opts := protogen.Options{ParamFunc: flags.Set}
	opts.Run(h.Run)
}
