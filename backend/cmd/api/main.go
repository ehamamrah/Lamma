// Command api runs the Lamma HTTP API.
//
//	api                 serve HTTP (default)
//	api migrate up|down apply or roll back database migrations
//	api healthcheck     probe /health/live (container healthcheck)
package main

import (
	"context"
	"os"

	"github.com/ehamamrah/lamma/backend/internal/app"
)

func main() {
	os.Exit(app.Main(context.Background(), os.Args[1:]))
}
