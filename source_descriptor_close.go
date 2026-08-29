//go:build !windows

package starter

import "os"

func closeSourceDescriptor(file *os.File, _ uintptr) error {
	return file.Close()
}
