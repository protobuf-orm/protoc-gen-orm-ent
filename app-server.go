package main

import (
	"context"
	"fmt"
	"text/template"

	"github.com/protobuf-orm/protobuf-orm/graph"
	"github.com/protobuf-orm/protoc-gen-orm-ent/apps/server/app"
	"google.golang.org/protobuf/compiler/protogen"
)

type ServerOpts struct {
	Namer string

	ent protogen.GoImportPath
}

func (h *ServerOpts) Run(ctx context.Context, p *protogen.Plugin, g *graph.Graph) error {
	opts := []app.Option{}
	if v, err := template.New("namer").Parse(h.Namer); err != nil {
		return fmt.Errorf("opt.server.namer: %w", err)
	} else {
		opts = append(opts, app.WithNamer(v))
	}

	app, err := app.New(h.ent, opts...)
	if err != nil {
		return fmt.Errorf("initialize server app: %w", err)
	}
	if err := app.Run(ctx, p, g); err != nil {
		return fmt.Errorf("run server app: %w", err)
	}

	return nil
}
