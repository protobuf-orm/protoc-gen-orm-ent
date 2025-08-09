package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/protobuf-orm/protobuf-orm/graph"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

type Handler struct {
	Schema SchemaOpts
	Ent    EntOpts
	Server ServerOpts
	Store  StoreOpts
}

func (h *Handler) Run(p *protogen.Plugin) error {
	p.SupportedEditionsMinimum = descriptorpb.Edition_EDITION_PROTO2
	p.SupportedEditionsMaximum = descriptorpb.Edition_EDITION_MAX
	p.SupportedFeatures = uint64(0 |
		pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL |
		pluginpb.CodeGeneratorResponse_FEATURE_SUPPORTS_EDITIONS,
	)

	ctx := context.Background()
	// TODO: set logger

	if v, err := template.New("namer").Parse(h.Ent.Namer); err != nil {
		return fmt.Errorf("opt.ent.namer: %w", err)
	} else {
		b := strings.Builder{}
		if err := v.Execute(&b, struct{ Name string }{Name: "_"}); err != nil {
			return fmt.Errorf("opt.ent.namer: %w", err)
		}

		path := b.String() // path/to/package/_.g.go
		pkg_user_ent := protogen.GoImportPath(filepath.Dir(path))
		h.Server.ent = pkg_user_ent
		h.Store.ent = pkg_user_ent
	}

	g := graph.NewGraph()
	for _, f := range p.Files {
		if err := graph.Parse(ctx, g, f.Desc); err != nil {
			return fmt.Errorf("parse entity at %s: %w", *f.Proto.Name, err)
		}
	}

	h.Schema.Run(ctx, p, g)
	h.Ent.Run(ctx, p, g)
	h.Server.Run(ctx, p, g)
	h.Store.Run(ctx, p, g)

	return nil
}
