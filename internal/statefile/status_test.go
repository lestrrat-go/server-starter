package statefile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteStatusFile(t *testing.T) {
	t.Run("sorts entries ascending by generation", func(t *testing.T) {
		dir := t.TempDir()
		fn := filepath.Join(dir, "status")

		err := WriteStatus(fn, map[int]int{3: 300, 1: 100, 2: 200})
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

		err := WriteStatus(fn, map[int]int{})
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

		err := WriteStatus("", map[int]int{1: 100})
		require.NoError(t, err)

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		require.Empty(t, entries)
	})

	t.Run("overwrites an existing file atomically and leaves no temp file", func(t *testing.T) {
		dir := t.TempDir()
		fn := filepath.Join(dir, "status")
		require.NoError(t, os.WriteFile(fn, []byte("stale\n"), 0644))

		err := WriteStatus(fn, map[int]int{1: 100})
		require.NoError(t, err)

		got, err := os.ReadFile(fn)
		require.NoError(t, err)
		require.Equal(t, "1:100\n", string(got))

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		require.Len(t, entries, 1, "no stray temporary file should remain")
	})
}

func TestReadStatus(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status")
	require.NoError(t, os.WriteFile(path, []byte("2:200\n1:100\n"), 0600))

	status, err := ReadStatus(path)
	require.NoError(t, err)
	require.Equal(t, map[int]int{1: 100, 2: 200}, status)
}

func TestStatusMap(t *testing.T) {
	t.Run("merges old workers and the current worker", func(t *testing.T) {
		oldWorkers := map[int]int{100: 1, 101: 2}

		got := StatusMap(oldWorkers, 999, 3)

		require.Equal(t, map[int]int{1: 100, 2: 101, 3: 999}, got)
	})

	t.Run("zero currentPID means no current worker", func(t *testing.T) {
		oldWorkers := map[int]int{100: 1}

		got := StatusMap(oldWorkers, 0, 2)

		require.Equal(t, map[int]int{1: 100}, got)
	})

	t.Run("no old workers, only the current one", func(t *testing.T) {
		got := StatusMap(map[int]int{}, 999, 0)

		require.Equal(t, map[int]int{0: 999}, got)
	})
}
