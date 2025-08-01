package main

import (
	"context"
	"fmt"
	"text/template"

	"github.com/protobuf-orm/protobuf-orm/graph"
	"github.com/protobuf-orm/protoc-gen-orm-ent/app"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

type Handler struct {
	Namer string
}

func (h *Handler) Run(p *protogen.Plugin) error {
	p.SupportedEditionsMinimum = descriptorpb.Edition_EDITION_PROTO2
	p.SupportedEditionsMaximum = descriptorpb.Edition_EDITION_MAX
	p.SupportedFeatures = uint64(0 |
		pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL |
		pluginpb.CodeGeneratorResponse_FEATURE_SUPPORTS_EDITIONS,
	)

	opts := []app.Option{}
	if h.Namer != "" {
		v, err := template.New("namer").Parse(h.Namer)
		if err != nil {
			return fmt.Errorf("opt.namer: %w", err)
		}
		opts = append(opts, app.WithNamer(v))
	}

	app, err := app.New(opts...)
	if err != nil {
		return fmt.Errorf("initialize plugin: %w", err)
	}

	ctx := context.Background()
	// TODO: set logger

	g := graph.NewGraph()
	for _, f := range p.Files {
		if err := graph.Parse(ctx, g, f.Desc); err != nil {
			return fmt.Errorf("parse entity at %s: %w", *f.Proto.Name, err)
		}
	}

	return app.Run(ctx, p, g)
}
