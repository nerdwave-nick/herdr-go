# herdr-go

Go client for the [Herdr](https://herdr.dev) socket API. Standard library only.

```go
c, err := herdr.FromEnv() // HERDR_SOCKET_PATH, set for plugins and Herdr panes
edges, err := c.PaneEdges(ctx, api.PaneEdgesParams{PaneID: id})

for ev, err := range c.Subscribe(ctx, api.SubscriptionPaneCreated{}, api.SubscriptionLayoutUpdated{}) {
	if err != nil { /* herdr.ErrEventsLost: resubscribe and reload */ }
	switch d := ev.Data.(type) {
	case api.EventPaneCreated:
	case api.EventLayoutUpdated:
	}
}
```

## Packages

- `api`: generated from the schema Herdr ships (`herdr api schema --json`).
  One typed method per request on `api.Client` (embedded in `herdr.Client`),
  structs and string enums for every definition, tagged unions as a wrapper
  struct holding a sealed interface (`api.LayoutNode{Value: api.LayoutNodeSplit{...}}`).
  Unknown union variants decode into `<Union>Unknown`, so newer servers do not
  break older clients. Every response checks its `type` discriminator.
- `herdr`: socket transport (`Call`, `Subscribe` as an `iter.Seq2`), `Error`
  with `HasCode(err, "pane_not_found")`.
- `herdrtest`: `herdrtest.Server(t)` starts an isolated headless Herdr server
  for a test (own XDG dirs) and stops it afterwards.

## Regenerating

```sh
herdr api schema --output schema/herdr-api.schema.json
go generate ./api
go test ./...
```

The schema does not link methods to responses; `internal/gen/responses.go` does,
with the entries verified against a live server marked. `TestLiveProtocolMatches`
fails when the installed Herdr speaks a different protocol than `api.Protocol`.

## Planned

`state`: declare the parts of Herdr's state a program needs as embedded facets
(`herdr.LivePanes`, ...), load them once, keep them current from events, and
get typed callbacks (`OnPaneCreated`) only for declared live facets, checked at
compile time. Callbacks run serially; each event yields a new immutable state
revision, so handing state to a goroutine is safe.
