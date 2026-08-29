//go:build linux

package supervisor

import (
	"debug/elf"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTerminalWorkerStartErrorRecognizesLinuxLaunchErrors(t *testing.T) {
	tests := map[string]error{
		"directory interpreter":   syscall.EISDIR,
		"unsupported interpreter": syscall.ELIBBAD,
	}

	for name, startErr := range tests {
		t.Run(name, func(t *testing.T) {
			err := fmt.Errorf("wrapped start error: %w", &os.PathError{
				Op:   "fork/exec",
				Path: "worker",
				Err:  startErr,
			})

			require.True(t, terminalWorkerStartError("worker", "", err))
			require.ErrorIs(t, err, startErr)
		})
	}
}

func TestUnsupportedELFInterpreterStopsWorkerStartRetries(t *testing.T) {
	dir := t.TempDir()
	const interpreterName = "unsupported-loader"

	interpreter := filepath.Join(dir, interpreterName)
	writeELFWithUnsupportedMachine(t, "/bin/sh", interpreter)

	worker := filepath.Join(dir, "worker")
	rewriteELFInterpreter(t, "/bin/sh", worker, interpreterName)
	requireSingleTerminalStartAttempt(t, config{command: worker, dir: dir}, nil, syscall.ELIBBAD)
}

func TestInvalidELFInterpreterStopsWorkerStartRetries(t *testing.T) {
	dir := t.TempDir()
	const interpreterName = "bad-loader"

	interpreter := filepath.Join(dir, interpreterName)
	require.NoError(t, os.WriteFile(interpreter, nil, 0o700))

	worker := filepath.Join(dir, "worker")
	rewriteELFInterpreter(t, "/bin/sh", worker, interpreterName)
	requireSingleTerminalStartAttempt(t, config{command: worker, dir: dir}, nil, syscall.EIO)
}

func TestTruncatedELFInterpreterStopsWorkerStartRetries(t *testing.T) {
	dir := t.TempDir()
	const interpreterName = "truncated-loader"

	interpreter := filepath.Join(dir, interpreterName)
	require.NoError(t, os.WriteFile(interpreter, []byte(elf.ELFMAG), 0o700))

	worker := filepath.Join(dir, "worker")
	rewriteELFInterpreter(t, "/bin/sh", worker, interpreterName)
	requireSingleTerminalStartAttempt(t, config{command: worker, dir: dir}, nil, syscall.EIO)
}

func TestELFReadFailureDoesNotStopWorkerStartRetries(t *testing.T) {
	require.False(t, malformedELF(failingReaderAt{err: syscall.EIO}))
}

func TestUnrelatedEIODoesNotStopWorkerStartRetries(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)

	startErr := &os.PathError{Op: "fork/exec", Path: executable, Err: syscall.EIO}
	require.False(t, terminalWorkerStartError(executable, "", startErr))
}

func rewriteELFInterpreter(t *testing.T, source, destination, interpreter string) {
	t.Helper()

	file, err := elf.Open(source)
	require.NoError(t, err)
	defer file.Close()

	var segment *elf.Prog
	for _, program := range file.Progs {
		if program.Type == elf.PT_INTERP {
			segment = program
			break
		}
	}
	require.NotNil(t, segment, "%s has no ELF interpreter", source)
	require.LessOrEqual(t, uint64(len(interpreter)+1), segment.Filesz)

	data, err := os.ReadFile(source)
	require.NoError(t, err)
	start := int(segment.Off)
	end := start + int(segment.Filesz)
	require.LessOrEqual(t, end, len(data))
	clear(data[start:end])
	copy(data[start:end], interpreter)
	require.NoError(t, os.WriteFile(destination, data, 0o700))
}

func writeELFWithUnsupportedMachine(t *testing.T, source, destination string) {
	t.Helper()

	file, err := elf.Open(source)
	require.NoError(t, err)
	machine := elf.EM_ARM
	if file.Machine == machine {
		machine = elf.EM_X86_64
	}
	byteOrder := file.ByteOrder
	require.NoError(t, file.Close())

	data, err := os.ReadFile(source)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(data), 20)
	byteOrder.PutUint16(data[18:20], uint16(machine))
	require.NoError(t, os.WriteFile(destination, data, 0o700))
}

type failingReaderAt struct {
	err error
}

func (r failingReaderAt) ReadAt([]byte, int64) (int, error) {
	return 0, r.err
}
