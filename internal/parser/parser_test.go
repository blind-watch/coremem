package parser_test

import (
	"testing"

	"coremem/internal/memory"
	"coremem/internal/parser"
)

func TestParseBlocks(t *testing.T) {
	text := `[coremem:type=core_constraint scope=repo title="No in-memory queues"]
Do not use in-memory queues for jobs.
[/coremem]`
	blocks, err := parser.ParseBlocks(text)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 1 {
		t.Fatalf("got %d blocks, want 1", len(blocks))
	}
	if blocks[0].Type != memory.TypeCoreConstraint || blocks[0].Scope != memory.ScopeRepo || blocks[0].Title != "No in-memory queues" {
		t.Fatalf("unexpected block: %+v", blocks[0])
	}
}

func TestParseBlocksRejectsInvalidType(t *testing.T) {
	_, err := parser.ParseBlocks(`[coremem:type=raw_transcript title="Bad"]body[/coremem]`)
	if err == nil {
		t.Fatal("expected invalid type error")
	}
}
