package starter

import (
	"fmt"
	"os"
	"sort"
)

// statusMap builds the generation-to-pid map that mirrors the current
// status-file contents: one entry for each old worker still draining
// (oldWorkers is keyed pid -> generation), plus one more for the current
// worker's pid if currentPID is non-zero.
func statusMap(oldWorkers map[int]int, currentPID int, generation int) map[int]int {
	m := make(map[int]int, len(oldWorkers)+1)
	for pid, gen := range oldWorkers {
		m[gen] = pid
	}
	if currentPID != 0 {
		m[generation] = currentPID
	}
	return m
}

// writeStatusFile writes fn to list each generation's worker pid, one
// "generation:pid" line per entry, sorted ascending by generation. An empty
// fn means "write nothing" and is a no-op.
//
// The file is written to a temporary path alongside fn and then renamed into
// place, so a concurrent reader of fn never observes a partially written
// file.
func writeStatusFile(fn string, generations map[int]int) error {
	if fn == "" {
		return nil
	}

	gens := make([]int, 0, len(generations))
	for gen := range generations {
		gens = append(gens, gen)
	}
	sort.Ints(gens)

	tmpfn := fmt.Sprintf("%s.%d", fn, os.Getpid())
	f, err := os.OpenFile(tmpfn, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to create temporary status file %q: %w", tmpfn, err)
	}

	for _, gen := range gens {
		if _, err := fmt.Fprintf(f, "%d:%d\n", gen, generations[gen]); err != nil {
			f.Close()
			os.Remove(tmpfn)
			return fmt.Errorf("failed to write temporary status file %q: %w", tmpfn, err)
		}
	}

	if err := f.Close(); err != nil {
		os.Remove(tmpfn)
		return fmt.Errorf("failed to close temporary status file %q: %w", tmpfn, err)
	}

	if err := os.Rename(tmpfn, fn); err != nil {
		os.Remove(tmpfn)
		return fmt.Errorf("failed to rename %q to %q: %w", tmpfn, fn, err)
	}

	return nil
}
