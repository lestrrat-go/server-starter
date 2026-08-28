//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package supervisor

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func moveToQuarantineAt(oldDir *os.File, oldName string, newDir *os.File, newName string) error {
	return unix.Renameat(int(oldDir.Fd()), oldName, int(newDir.Fd()), newName)
}

// renameNoReplaceByLinkAt restores an entry from the verified private
// quarantine. Link provides the no-replace step; the anchored identity check
// prevents a renamed quarantine pathname from causing removal of another
// source.
func renameNoReplaceByLinkAt(oldDir *os.File, oldName string, newDir *os.File, newName string) error {
	oldPath := filepath.Join(oldDir.Name(), oldName)
	newPath := filepath.Join(newDir.Name(), newName)
	if err := os.Link(oldPath, newPath); err != nil {
		return err
	}

	var oldStat unix.Stat_t
	if err := unix.Fstatat(int(oldDir.Fd()), oldName, &oldStat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return fmt.Errorf("inspect quarantine source after link: %w", err)
	}
	var newStat unix.Stat_t
	if err := unix.Fstatat(int(newDir.Fd()), newName, &newStat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return fmt.Errorf("inspect restored path after link: %w", err)
	}
	if oldStat.Dev != newStat.Dev || oldStat.Ino != newStat.Ino {
		return fmt.Errorf("quarantine or destination pathname changed during restore")
	}
	return unix.Unlinkat(int(oldDir.Fd()), oldName, 0)
}

func pathIsSocketAt(dir *os.File, name string) (bool, error) {
	var stat unix.Stat_t
	if err := unix.Fstatat(int(dir.Fd()), name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return false, err
	}
	return stat.Mode&unix.S_IFMT == unix.S_IFSOCK, nil
}

func removeAt(dir *os.File, name string) error {
	return unix.Unlinkat(int(dir.Fd()), name, 0)
}

func createPrivateDirAt(dir *os.File, name string) (*os.File, error) {
	return createPrivateDirAtWithOpen(dir, name, openPrivateDirAt)
}

func createPrivateDirAtWithOpen(
	dir *os.File,
	name string,
	openDir func(*os.File, string) (*os.File, error),
) (*os.File, error) {
	dirFD := int(dir.Fd())
	if err := unix.Mkdirat(dirFD, name, 0700); err != nil {
		return nil, err
	}

	privateDir, err := openDir(dir, name)
	if err != nil {
		_ = unix.Unlinkat(dirFD, name, unix.AT_REMOVEDIR)
		return nil, err
	}
	if err := verifyPrivateDir(privateDir); err != nil {
		_ = privateDir.Close()
		_ = unix.Unlinkat(dirFD, name, unix.AT_REMOVEDIR)
		return nil, err
	}
	return privateDir, nil
}

func openPrivateDirAt(dir *os.File, name string) (*os.File, error) {
	fd, err := unix.Openat(
		int(dir.Fd()),
		name,
		unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW,
		0,
	)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), filepath.Join(dir.Name(), name)), nil
}

func verifyPrivateDir(dir *os.File) error {
	var stat unix.Stat_t
	if err := unix.Fstat(int(dir.Fd()), &stat); err != nil {
		return err
	}
	if stat.Uid != uint32(os.Geteuid()) {
		return fmt.Errorf("quarantine directory is owned by uid %d, want %d", stat.Uid, os.Geteuid())
	}
	if permissions := stat.Mode & 0777; permissions != 0700 {
		return fmt.Errorf("quarantine directory permissions are %#o, want 0700", permissions)
	}
	return nil
}

func removeDirAt(dir *os.File, name string) error {
	return unix.Unlinkat(int(dir.Fd()), name, unix.AT_REMOVEDIR)
}
