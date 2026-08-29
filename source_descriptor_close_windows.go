package starter

import (
	"os"
	"syscall"
)

func closeSourceDescriptor(file *os.File, socket uintptr) error {
	// Retire the NewFile wrapper before closing the socket with the Winsock API.
	// NewFile uses CloseHandle, which does not close a Windows socket.
	_ = file.Close()
	return syscall.Closesocket(syscall.Handle(socket))
}
