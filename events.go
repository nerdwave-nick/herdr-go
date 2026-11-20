package herdr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"time"

	"github.com/nerdwave-nick/herdr-go/api"
)

// Event is one pushed event.
type Event struct {
	// Kind is the envelope's event name, e.g. "pane_created" or
	// "pane.output_matched".
	Kind string
	// Data is the decoded payload. Lifecycle events are an
	// api.EventDataVariant (api.EventPaneCreated, api.EventTabMoved, ...,
	// or api.EventDataUnknown for kinds newer than this package).
	// Subscription-only events are their own types, e.g.
	// api.PaneOutputMatchedEvent.
	Data any
	// Raw is the undecoded data object.
	Raw json.RawMessage
}

var noDeadline time.Time

// ErrEventsLost is returned by Subscribe when the server dropped events
// because the subscriber fell behind. State derived from events is stale:
// resubscribe and reload.
var ErrEventsLost = errors.New("herdr: events lost; resubscribe and reload state")

// Subscribe opens a subscription and yields events until ctx is done, the
// connection fails, or the server reports lost events (ErrEventsLost). The
// subscription is acknowledged before the first event is yielded; a failed
// acknowledgement is yielded as an error.
//
//	for ev, err := range c.Subscribe(ctx, api.SubscriptionPaneCreated{}, api.SubscriptionTabMoved{}) {
//		if err != nil { ... }
//		switch d := ev.Data.(type) {
//		case api.EventPaneCreated: ...
//		}
//	}
func (c *Client) Subscribe(ctx context.Context, subs ...api.SubscriptionVariant) iter.Seq2[Event, error] {
	wrapped := make([]api.Subscription, len(subs))
	for i, s := range subs {
		wrapped[i] = api.Subscription{Value: s}
	}
	return func(yield func(Event, error) bool) {
		conn, r, err := c.send(ctx, "events.subscribe", api.EventsSubscribeParams{Subscriptions: wrapped})
		if err != nil {
			yield(Event{}, err)
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(noDeadline)

		for {
			line, err := r.ReadBytes('\n')
			if err != nil {
				if len(line) == 0 {
					if ctx.Err() != nil {
						return
					}
					yield(Event{}, fmt.Errorf("herdr: subscription: %w", err))
					return
				}
			}
			ev, done, err := decodeLine(trimLine(line))
			if err != nil {
				yield(Event{}, err)
				return
			}
			if done {
				continue // acknowledgement
			}
			if !yield(ev, nil) {
				return
			}
		}
	}
}

// decodeLine decodes one subscription line. ack is true for the
// acknowledgement response.
func decodeLine(line []byte) (ev Event, ack bool, err error) {
	var probe struct {
		ID     *string         `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  *Error          `json:"error"`
		Event  string          `json:"event"`
		Data   json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(line, &probe); err != nil {
		return ev, false, fmt.Errorf("herdr: subscription: decode: %w", err)
	}
	if probe.Error != nil {
		probe.Error.Method = "events.subscribe"
		if probe.Error.Code == "events_lost" {
			return ev, false, errors.Join(ErrEventsLost, probe.Error)
		}
		return ev, false, probe.Error
	}
	if probe.ID != nil && probe.Event == "" {
		return ev, true, nil
	}
	ev = Event{Kind: probe.Event, Raw: probe.Data}
	if v, ok, err := api.DecodeSubscriptionEvent(probe.Event, probe.Data); ok {
		if err != nil {
			return ev, false, fmt.Errorf("herdr: event %s: %w", probe.Event, err)
		}
		ev.Data = v
		return ev, false, nil
	}
	var d api.EventData
	if err := json.Unmarshal(probe.Data, &d); err != nil {
		return ev, false, fmt.Errorf("herdr: event %s: %w", probe.Event, err)
	}
	ev.Data = d.Value
	return ev, false, nil
}
