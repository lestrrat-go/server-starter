package control

import (
	"testing"
)

func TestGenerationTransitions(t *testing.T) {
	status := map[int]int{1: 100, 2: 200}
	if !generationAdvanced(map[int]int{1: 100}, status) {
		t.Fatal("status did not advance")
	}
	if !oldWorkersGone(map[int]int{1: 100}, map[int]int{2: 200}) {
		t.Fatal("old worker was reported as alive")
	}
}
