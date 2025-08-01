package app

import "text/template"

type Option func(a *App)

func WithNamer(v *template.Template) Option {
	return func(a *App) {
		a.namer = v
	}
}
