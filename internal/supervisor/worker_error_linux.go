//go:build linux

package supervisor

import (
	"bytes"
	"debug/elf"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

func platformTerminalWorkerStartError(command, dir string, err error) bool {
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) || pathErr.Op != "fork/exec" || !errors.Is(err, syscall.EIO) {
		return false
	}

	// Linux also reports transient executable-filesystem failures as EIO.
	// Stop only when both files are readable and the ELF interpreter is
	// demonstrably malformed.
	interpreter, ok := workerELFInterpreter(command, dir)
	if !ok {
		return false
	}
	if !filepath.IsAbs(interpreter) {
		interpreter = filepath.Join(dir, interpreter)
	}

	file, openErr := os.Open(interpreter)
	if openErr != nil {
		return false
	}
	defer file.Close()

	return malformedELF(file)
}

func malformedELF(reader io.ReaderAt) bool {
	_, err := elf.NewFile(reader)
	if err == nil {
		return false
	}

	var formatErr *elf.FormatError
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.As(err, &formatErr)
}

// workerELFInterpreter returns the interpreter path recorded in a readable
// ELF executable. An inspection failure provides no evidence that EIO is
// permanent, so callers leave the launch retryable.
func workerELFInterpreter(command, dir string) (string, bool) {
	if !filepath.IsAbs(command) && dir != "" {
		command = filepath.Join(dir, command)
	}

	file, err := elf.Open(command)
	if err != nil {
		return "", false
	}
	defer file.Close()

	for _, program := range file.Progs {
		if program.Type != elf.PT_INTERP {
			continue
		}

		data, err := io.ReadAll(program.Open())
		if err != nil {
			return "", false
		}
		terminator := bytes.IndexByte(data, 0)
		if terminator <= 0 {
			return "", false
		}
		return string(data[:terminator]), true
	}
	return "", false
}
