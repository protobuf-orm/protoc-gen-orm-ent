package app

import (
	"text/template"
)

type Option func(a *App)

func WithNamer(v *template.Template) Option {
	return func(a *App) {
		a.namer = v
	}
}

// WithClientName names the one file that is about the package rather than
// about an entity. It is placed beside the files [WithNamer] names.
func WithClientName(v string) Option {
	return func(a *App) {
		a.client = v
	}
}
