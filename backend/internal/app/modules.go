package app

import "github.com/go-chi/chi/v5"

// buildModules constructs every business module with its dependencies and
// returns their public routes, internal routes and background workers.
func buildModules(_ deps) ([]module, func(chi.Router), []Worker) {
	return nil, func(chi.Router) {}, nil
}
