package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/protobuf-orm/protobuf-orm/graph"
	"github.com/protobuf-orm/protoc-gen-orm-ent/apps/server/app"
	"google.golang.org/protobuf/compiler/protogen"
)

func (h *Handler) runServerApp(ctx context.Context, p *protogen.Plugin, g *graph.Graph) error {
	opts := []app.Option{}
	if v, err := template.New("namer").Parse(h.NamerServer); err != nil {
		return fmt.Errorf("opt.namer-server: %w", err)
	} else {
		opts = append(opts, app.WithNamer(v))
	}

	b := strings.Builder{}
	if v, err := template.New("namer").Parse(h.NamerEnt); err != nil {
		return fmt.Errorf("opt.namer-ent: %w", err)
	} else if err := v.Execute(&b, struct{ Name string }{Name: "_"}); err != nil {
		return fmt.Errorf("opt.namer-ent: %w", err)
	}

	path := b.String() // path/to/package/_.g.go
	ent := filepath.Dir(path)

	app, err := app.New(protogen.GoImportPath(ent), opts...)
	if err != nil {
		return fmt.Errorf("initialize schema app: %w", err)
	}
	if err := app.Run(ctx, p, g); err != nil {
		return fmt.Errorf("run schema app: %w", err)
	}

	return nil
}
