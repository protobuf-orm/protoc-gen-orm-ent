package main

import (
	"context"
	"fmt"
	"text/template"

	"github.com/protobuf-orm/protobuf-orm/graph"
	"github.com/protobuf-orm/protoc-gen-orm-ent/apps/schema/app"
	"google.golang.org/protobuf/compiler/protogen"
)

type SchemaOpts struct {
	Namer string
}

func (h *SchemaOpts) Run(ctx context.Context, p *protogen.Plugin, g *graph.Graph) error {
	opts := []app.Option{}
	if h.Namer != "" {
		v, err := template.New("namer").Parse(h.Namer)
		if err != nil {
			return fmt.Errorf("opt.namer-schema: %w", err)
		}
		opts = append(opts, app.WithNamer(v))
	}

	app, err := app.New(opts...)
	if err != nil {
		return fmt.Errorf("initialize schema app: %w", err)
	}
	if err := app.Run(ctx, p, g); err != nil {
		return fmt.Errorf("run schema app: %w", err)
	}

	return nil
}
