package statefile

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// StatusMap builds the generation-to-pid map that mirrors the current
// status-file contents: one entry for each old worker still draining
// (oldWorkers is keyed pid -> generation), plus one more for the current
// worker's pid if currentPID is non-zero.
func StatusMap(oldWorkers map[int]int, currentPID int, generation int) map[int]int {
	m := make(map[int]int, len(oldWorkers)+1)
	for pid, gen := range oldWorkers {
		m[gen] = pid
	}
	if currentPID != 0 {
		m[generation] = currentPID
	}
	return m
}

// WriteStatus writes fn to list each generation's worker pid, one
// "generation:pid" line per entry, sorted ascending by generation. An empty
// fn means "write nothing" and is a no-op.
//
// The file is written to a temporary path alongside fn and then renamed into
// place, so a concurrent reader of fn never observes a partially written
// file.
func WriteStatus(fn string, generations map[int]int) error {
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

// ReadPID reads and parses the pid stored in the pid file at path.
func ReadPID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	value := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(value)
	if err != nil || pid <= 0 {
		return 0, fmt.Errorf("invalid pid file %q", path)
	}
	return pid, nil
}

// ReadStatus reads and parses the status file at path into a
// generation-to-pid map, one entry per "generation:pid" line.
func ReadStatus(path string) (map[int]int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	status := make(map[int]int)
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid status line %q", line)
		}
		generation, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, err
		}
		pid, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, err
		}
		status[generation] = pid
	}
	return status, nil
}
