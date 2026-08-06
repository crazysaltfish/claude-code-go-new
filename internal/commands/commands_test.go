package commands

import "testing"

func TestRegistryPreservesCommandOrderAndReplacement(t *testing.T) {
	registry := NewRegistry()
	registry.Register(NewClearCommand())
	registry.Register(NewCostCommand())
	registry.Register(NewClearCommand())

	got := registry.ListEnabled()
	if len(got) != 2 {
		t.Fatalf("registry contains %d commands, want 2", len(got))
	}
	if got[0].Name() != "clear" || got[1].Name() != "cost" {
		t.Fatalf("command order = [%s, %s], want [clear, cost]", got[0].Name(), got[1].Name())
	}
}
