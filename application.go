package docker

type Application struct {
	container Container
	hooks     []Hook
}

func NewApplication(c Container, hooks ...Hook) *Application {
	return &Application{
		container: c,
		hooks:     hooks,
	}
}
