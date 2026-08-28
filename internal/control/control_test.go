package control

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerationTransitions(t *testing.T) {
	status := map[int]int{1: 100, 2: 200}
	require.True(t, generationAdvanced(map[int]int{1: 100}, status), "status did not advance")
	require.True(t, oldWorkersGone(map[int]int{1: 100}, map[int]int{2: 200}), "old worker was reported as alive")
}
