package supervisor

import "golang.org/x/sys/unix"

func openQuarantineDirectory(path string) (int, error) {
	// O_EVTONLY provides a directory descriptor without requiring read access.
	return unix.Open(path, unix.O_EVTONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
}

func openQuarantineDirectoryAt(parentFD int, name string) (int, error) {
	return unix.Openat(parentFD, name, unix.O_EVTONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
}

func openQuarantineSource(parentFD int, name string) (int, error) {
	return unix.Openat(parentFD, name, unix.O_EVTONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
}
