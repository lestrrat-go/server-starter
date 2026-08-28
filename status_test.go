package starter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// writeStatusFile and statusMap are unexported, so this test stays in the
// internal package rather than starter_test.

func TestWriteStatusFile(t *testing.T) {
	t.Run("sorts entries ascending by generation", func(t *testing.T) {
		dir := t.TempDir()
		fn := filepath.Join(dir, "status")

		err := writeStatusFile(fn, map[int]int{3: 300, 1: 100, 2: 200})
		require.NoError(t, err)

		got, err := os.ReadFile(fn)
		require.NoError(t, err)
		require.Equal(t, "1:100\n2:200\n3:300\n", string(got))

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		require.Len(t, entries, 1, "no stray temporary file should remain")
		require.Equal(t, "status", entries[0].Name())
	})

	t.Run("empty generations map writes empty file", func(t *testing.T) {
		dir := t.TempDir()
		fn := filepath.Join(dir, "status")

		err := writeStatusFile(fn, map[int]int{})
		require.NoError(t, err)

		got, err := os.ReadFile(fn)
		require.NoError(t, err)
		require.Empty(t, got)

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		require.Len(t, entries, 1, "no stray temporary file should remain")
	})

	t.Run("empty path writes nothing", func(t *testing.T) {
		dir := t.TempDir()

		err := writeStatusFile("", map[int]int{1: 100})
		require.NoError(t, err)

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		require.Empty(t, entries)
	})

	t.Run("overwrites an existing file atomically and leaves no temp file", func(t *testing.T) {
		dir := t.TempDir()
		fn := filepath.Join(dir, "status")
		require.NoError(t, os.WriteFile(fn, []byte("stale\n"), 0644))

		err := writeStatusFile(fn, map[int]int{1: 100})
		require.NoError(t, err)

		got, err := os.ReadFile(fn)
		require.NoError(t, err)
		require.Equal(t, "1:100\n", string(got))

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		require.Len(t, entries, 1, "no stray temporary file should remain")
	})
}

func TestStatusMap(t *testing.T) {
	t.Run("merges old workers and the current worker", func(t *testing.T) {
		oldWorkers := map[int]int{100: 1, 101: 2}

		got := statusMap(oldWorkers, 999, 3)

		require.Equal(t, map[int]int{1: 100, 2: 101, 3: 999}, got)
	})

	t.Run("zero currentPID means no current worker", func(t *testing.T) {
		oldWorkers := map[int]int{100: 1}

		got := statusMap(oldWorkers, 0, 2)

		require.Equal(t, map[int]int{1: 100}, got)
	})

	t.Run("no old workers, only the current one", func(t *testing.T) {
		got := statusMap(map[int]int{}, 999, 0)

		require.Equal(t, map[int]int{0: 999}, got)
	})
}
