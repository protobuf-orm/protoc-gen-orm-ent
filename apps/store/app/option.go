package app

type Option func(a *App)

func WithName(v string) Option {
	return func(a *App) {
		a.name = v
	}
}
