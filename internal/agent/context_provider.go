package agent

import "github.com/thalysguimaraes/cliphub/internal/privacy"

type contextProvider interface {
	CurrentContext() (privacy.Context, error)
}

type noopContextProvider struct{}

func (noopContextProvider) CurrentContext() (privacy.Context, error) {
	return privacy.Context{}, nil
}
