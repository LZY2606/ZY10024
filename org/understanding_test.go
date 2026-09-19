package org

import (
	"strings"
	"testing"
)

func TestUnderstandingUnknownBlockPreservesWrapperAcrossOrgRoundTrip(t *testing.T) {
	input := "#+BEGIN_PUZZLE\nvalue\n#+END_PUZZLE\n"
	document := New().Silent().Parse(strings.NewReader(input), "understanding.org")
	if document.Error != nil {
		t.Fatalf("parse returned error: %v", document.Error)
	}

	block, ok := document.Nodes[0].(Block)
	if !ok {
		t.Fatalf("expected root node Block, got %T", document.Nodes[0])
	}
	if block.Name != "PUZZLE" {
		t.Fatalf("expected normalized block name PUZZLE, got %q", block.Name)
	}
	if _, ok := block.Children[0].(Paragraph); !ok {
		t.Fatalf("expected unknown block content to remain structured Paragraph, got %T", block.Children[0])
	}

	output, err := document.Write(NewOrgWriter())
	if err != nil {
		t.Fatalf("org write returned error: %v", err)
	}
	reparsed := New().Silent().Parse(strings.NewReader(output), "understanding-roundtrip.org")
	if reparsed.Error != nil {
		t.Fatalf("reparse returned error: %v", reparsed.Error)
	}
	roundTripBlock, ok := reparsed.Nodes[0].(Block)
	if !ok {
		t.Fatalf("expected reparsed root node Block, got %T", reparsed.Nodes[0])
	}
	if roundTripBlock.Name != "PUZZLE" {
		t.Fatalf("expected round-trip block name PUZZLE, got %q", roundTripBlock.Name)
	}
}
