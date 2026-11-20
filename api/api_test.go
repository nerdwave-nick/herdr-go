package api

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUnionMarshalInjectsDiscriminator(t *testing.T) {
	p := PaneMoveParams{
		PaneID:      "w1:p2",
		Destination: PaneMoveDestination{Value: PaneMoveDestinationTab{TabID: "w1:t2", Split: SplitDirectionRight}},
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"destination":{"type":"tab","split":"right","tab_id":"w1:t2"},"pane_id":"w1:p2"}`
	if string(b) != want {
		t.Fatalf("got  %s\nwant %s", b, want)
	}
}

func TestUnionRoundTripRecursive(t *testing.T) {
	in := `{"type":"split","direction":"right","ratio":0.5,
		"first":{"type":"pane","pane_id":"w1:p1"},
		"second":{"type":"split","direction":"down","ratio":0.3,
			"first":{"type":"pane","pane_id":"w1:p2"},"second":{"type":"pane","pane_id":"w1:p3"}}}`
	var n LayoutNode
	if err := json.Unmarshal([]byte(in), &n); err != nil {
		t.Fatal(err)
	}
	split, ok := n.Value.(LayoutNodeSplit)
	if !ok {
		t.Fatalf("root is %T, want LayoutNodeSplit", n.Value)
	}
	if p, ok := split.First.Value.(LayoutNodePane); !ok || p.PaneID != "w1:p1" {
		t.Fatalf("first = %#v", split.First.Value)
	}
	inner := split.Second.Value.(LayoutNodeSplit)
	if inner.Direction != SplitDirectionDown || inner.Second.Value.(LayoutNodePane).PaneID != "w1:p3" {
		t.Fatalf("inner = %#v", inner)
	}
	out, err := json.Marshal(n)
	if err != nil {
		t.Fatal(err)
	}
	var again LayoutNode
	if err := json.Unmarshal(out, &again); err != nil {
		t.Fatal(err)
	}
	if again.Value.(LayoutNodeSplit).Second.Value.(LayoutNodeSplit).Ratio != 0.3 {
		t.Fatal("round trip lost data")
	}
}

func TestUnknownVariantIsPreserved(t *testing.T) {
	in := `{"type":"pane_teleported","pane_id":"w1:p9"}`
	var d EventData
	if err := json.Unmarshal([]byte(in), &d); err != nil {
		t.Fatal(err)
	}
	u, ok := d.Value.(EventDataUnknown)
	if !ok || u.Type != "pane_teleported" {
		t.Fatalf("got %#v", d.Value)
	}
	out, _ := json.Marshal(d)
	if string(out) != in {
		t.Fatalf("unknown variant re-marshalled as %s", out)
	}
}

func TestResponseChecksDiscriminator(t *testing.T) {
	var r PaneLayoutResponse
	err := json.Unmarshal([]byte(`{"type":"pane_edges","edges":{}}`), &r)
	if err == nil || !strings.Contains(err.Error(), `want "pane_layout"`) {
		t.Fatalf("expected discriminator mismatch, got %v", err)
	}
}

func TestDefaultTrueBoolIsPointer(t *testing.T) {
	b, _ := json.Marshal(PaneReadParams{PaneID: "w1:p1", Source: ReadSourceVisible})
	if strings.Contains(string(b), "strip_ansi") {
		t.Fatalf("unset *bool must be omitted so the server default applies: %s", b)
	}
	b, _ = json.Marshal(PaneReadParams{PaneID: "w1:p1", Source: ReadSourceVisible, StripANSI: Ptr(false)})
	if !strings.Contains(string(b), `"strip_ansi":false`) {
		t.Fatalf("explicit false must be sent: %s", b)
	}
}
