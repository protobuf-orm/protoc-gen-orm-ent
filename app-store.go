package main

import (
	"context"
	"fmt"

	"github.com/protobuf-orm/protobuf-orm/graph"
	"github.com/protobuf-orm/protoc-gen-orm-ent/apps/store/app"
	"google.golang.org/protobuf/compiler/protogen"
)

type StoreOpts struct {
	Name string

	ent protogen.GoImportPath
	// TODO: currently, it is expected that the store will be placed in the same package with the server.
	// server protogen.GoImportPath
}

func (h *StoreOpts) Run(ctx context.Context, p *protogen.Plugin, g *graph.Graph) error {
	opts := []app.Option{}
	if h.Name != "" {
		opts = append(opts, app.WithName(h.Name))
	}

	app, err := app.New(h.ent, opts...)
	if err != nil {
		return fmt.Errorf("initialize store app: %w", err)
	}
	if err := app.Run(ctx, p, g); err != nil {
		return fmt.Errorf("run store app: %w", err)
	}

	return nil
}
