package herdr_test

import (
	"context"
	"errors"
	"testing"
	"time"

	herdr "github.com/nerdwave-nick/herdr-go"
	"github.com/nerdwave-nick/herdr-go/api"
	"github.com/nerdwave-nick/herdr-go/herdrtest"
)

func TestLiveProtocolMatches(t *testing.T) {
	c := herdrtest.Server(t)
	pong, err := c.Ping(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if pong.Protocol != api.Protocol {
		t.Fatalf("server protocol %d, package generated for %d: regenerate api", pong.Protocol, api.Protocol)
	}
}

// TestLiveResponses exercises the method->response mapping and decoding of
// nested types against a real server.
func TestLiveResponses(t *testing.T) {
	c := herdrtest.Server(t)
	ctx := context.Background()

	ws, err := c.WorkspaceCreate(ctx, api.WorkspaceCreateParams{Label: "t", Focus: true})
	if err != nil {
		t.Fatal(err)
	}
	p1 := ws.RootPane.PaneID
	split, err := c.PaneSplit(ctx, api.PaneSplitParams{TargetPaneID: p1, Direction: api.SplitDirectionRight})
	if err != nil {
		t.Fatal(err)
	}
	p2 := split.Pane.PaneID

	edges, err := c.PaneEdges(ctx, api.PaneEdgesParams{PaneID: p1})
	if err != nil {
		t.Fatal(err)
	}
	if !edges.Edges.Left || edges.Edges.Right || len(edges.Edges.Layout.Panes) != 2 {
		t.Fatalf("edges = %+v", edges.Edges)
	}

	exp, err := c.LayoutExport(ctx, api.LayoutExportParams{TabID: ws.Tab.TabID})
	if err != nil {
		t.Fatal(err)
	}
	root, ok := exp.Layout.Root.Value.(api.LayoutNodeSplit)
	if !ok || root.Direction != api.SplitDirectionRight {
		t.Fatalf("export root = %#v", exp.Layout.Root.Value)
	}

	tab, err := c.TabCreate(ctx, api.TabCreateParams{WorkspaceID: ws.Workspace.WorkspaceID})
	if err != nil {
		t.Fatal(err)
	}
	mv, err := c.PaneMove(ctx, api.PaneMoveParams{
		PaneID:      p2,
		Destination: api.PaneMoveDestination{Value: api.PaneMoveDestinationTab{TabID: tab.Tab.TabID, Split: api.SplitDirectionDown}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !mv.MoveResult.Changed || mv.MoveResult.Pane.TabID != tab.Tab.TabID {
		t.Fatalf("move = %+v", mv.MoveResult)
	}

	snap, err := c.SessionSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Snapshot.Workspaces) != 1 || len(snap.Snapshot.Tabs) != 2 || len(snap.Snapshot.Layouts) != 2 {
		t.Fatalf("snapshot: %d ws, %d tabs, %d layouts", len(snap.Snapshot.Workspaces), len(snap.Snapshot.Tabs), len(snap.Snapshot.Layouts))
	}

	_, err = c.PaneGet(ctx, api.PaneTarget{PaneID: "w1:p999"})
	if !herdr.HasCode(err, "pane_not_found") {
		t.Fatalf("want pane_not_found, got %v", err)
	}
}

func TestLiveSubscribe(t *testing.T) {
	c := herdrtest.Server(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ws, err := c.WorkspaceCreate(ctx, api.WorkspaceCreateParams{})
	if err != nil {
		t.Fatal(err)
	}

	events := c.Subscribe(ctx, api.SubscriptionPaneCreated{}, api.SubscriptionLayoutUpdated{})
	got := make(chan herdr.Event, 16)
	errc := make(chan error, 1)
	go func() {
		for ev, err := range events {
			if err != nil {
				errc <- err
				return
			}
			got <- ev
		}
	}()

	// The subscription is set up asynchronously; retry the trigger until an
	// event arrives so the test does not race the acknowledgement.
	var created api.EventPaneCreated
	var sawLayout bool
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	split := func() {
		_, _ = c.PaneSplit(ctx, api.PaneSplitParams{TargetPaneID: ws.RootPane.PaneID, Direction: api.SplitDirectionDown})
	}
	split()
	for created.Pane.PaneID == "" || !sawLayout {
		select {
		case ev := <-got:
			switch d := ev.Data.(type) {
			case api.EventPaneCreated:
				created = d
			case api.EventLayoutUpdated:
				sawLayout = len(d.Layout.Panes) > 0
			default:
				t.Fatalf("unexpected event %s: %T", ev.Kind, ev.Data)
			}
		case err := <-errc:
			t.Fatal(err)
		case <-tick.C:
			if created.Pane.PaneID == "" {
				split()
			}
		case <-ctx.Done():
			t.Fatalf("timed out: created=%v layout=%v", created.Pane.PaneID, sawLayout)
		}
	}
	if created.Pane.WorkspaceID != ws.Workspace.WorkspaceID {
		t.Fatalf("created pane in %s", created.Pane.WorkspaceID)
	}
}

func TestCallHonoursContext(t *testing.T) {
	c := herdr.New("/nonexistent/herdr.sock")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Ping(ctx); err == nil {
		t.Fatal("expected error")
	}
	var he *herdr.Error
	if errors.As(errors.New("x"), &he) {
		t.Fatal("unreachable")
	}
}
