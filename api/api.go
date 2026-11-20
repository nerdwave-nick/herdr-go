// Package api holds the Herdr socket API types and one typed function per
// request method, generated from the JSON Schema that Herdr ships
// (`herdr api schema --json`).
//
// The package is transport-agnostic: Client wraps any Caller and exposes one
// method per request. The herdr package provides a socket Caller and embeds
// Client, so the methods are available on it directly.
//
//	res, err := c.PaneLayout(ctx, api.PaneLayoutParams{PaneID: "w1:p1"})
//
// Regenerate after upgrading Herdr:
//
//	herdr api schema --output schema/herdr-api.schema.json
//	go generate ./api
package api

//go:generate go run ../internal/gen -schema ../schema/herdr-api.schema.json -out zz_generated.go

import "context"

// Caller sends one request and decodes the result into result.
type Caller interface {
	Call(ctx context.Context, method string, params, result any) error
}

// Client exposes every request method as a typed Go method.
type Client struct {
	Caller Caller
}

// Ptr returns a pointer to v, for optional fields such as *bool.
func Ptr[T any](v T) *T { return &v }
