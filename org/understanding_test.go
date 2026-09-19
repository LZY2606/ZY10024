package org

import (
	"strings"
	"testing"
)

func TestUnderstandingUnknownBlockRoundTrip(t *testing.T) {
	input := strings.NewReader("#+BEGIN_WIDGET\nBody with /emphasis/.\n#+END_WIDGET\n")
	first := New().Silent().Parse(input, "understanding.org")
	if first.Error != nil {
		t.Fatalf("first parse: %v", first.Error)
	}

	block, ok := first.Nodes[0].(Block)
	if !ok {
		t.Fatalf("first Nodes[0] = %T, want Block", first.Nodes[0])
	}
	if block.Name != "WIDGET" || len(block.Children) != 1 {
		t.Fatalf("block = Name %q with %d children; want WIDGET with 1 child", block.Name, len(block.Children))
	}
	paragraph, ok := block.Children[0].(Paragraph)
	if !ok || len(paragraph.Children) != 3 {
		t.Fatalf("block child = %#v, want one Paragraph with three inline children", block.Children[0])
	}
	if _, ok := paragraph.Children[1].(Emphasis); !ok {
		t.Fatalf("middle inline node = %T, want Emphasis", paragraph.Children[1])
	}

	orgOutput, err := first.Write(NewOrgWriter())
	if err != nil {
		t.Fatalf("write org: %v", err)
	}
	second := New().Silent().Parse(strings.NewReader(orgOutput), "understanding-roundtrip.org")
	if second.Error != nil {
		t.Fatalf("second parse: %v", second.Error)
	}

	roundTripBlock, ok := second.Nodes[0].(Block)
	if !ok {
		t.Fatalf("second Nodes[0] = %T, want Block", second.Nodes[0])
	}
	if roundTripBlock.Name != "WIDGET" || len(second.Nodes) != 1 || len(roundTripBlock.Children) != 1 {
		t.Fatalf("round trip = %d top nodes with %q and %d children; want one WIDGET block with one child",
			len(second.Nodes), roundTripBlock.Name, len(roundTripBlock.Children))
	}
}
