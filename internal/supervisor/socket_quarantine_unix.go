//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package supervisor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func moveToQuarantineAt(oldDir *os.File, oldName string, newDir *os.File, newName string) error {
	return unix.Renameat(int(oldDir.Fd()), oldName, int(newDir.Fd()), newName)
}

type pathIdentity struct {
	dev  uint64
	ino  uint64
	mode uint32
}

func pathIdentityAt(dir *os.File, name string) (pathIdentity, error) {
	var stat unix.Stat_t
	if err := unix.Fstatat(int(dir.Fd()), name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return pathIdentity{}, err
	}
	// Stat_t field widths vary across supported Unix targets.
	return pathIdentity{dev: uint64(stat.Dev), ino: uint64(stat.Ino), mode: uint32(stat.Mode)}, nil //nolint:unconvert
}

func pathIdentityForFile(file *os.File) (pathIdentity, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &stat); err != nil {
		return pathIdentity{}, err
	}
	// Stat_t field widths vary across supported Unix targets.
	return pathIdentity{dev: uint64(stat.Dev), ino: uint64(stat.Ino), mode: uint32(stat.Mode)}, nil //nolint:unconvert
}

func samePathIdentity(left, right pathIdentity) bool {
	return left.dev == right.dev && left.ino == right.ino &&
		left.mode&unix.S_IFMT == right.mode&unix.S_IFMT
}

func (identity pathIdentity) isSocket() bool {
	return identity.mode&unix.S_IFMT == unix.S_IFSOCK
}

func renameNoReplaceEntryAt(
	oldDir *os.File,
	oldName string,
	newDir *os.File,
	newName string,
	expected pathIdentity,
) error {
	current, err := pathIdentityAt(oldDir, oldName)
	if err != nil {
		return err
	}
	if !samePathIdentity(current, expected) {
		return fmt.Errorf("filesystem entry changed before restoration")
	}
	return renameNoReplaceAt(oldDir, oldName, newDir, newName)
}

// renameNoReplaceByLinkAt restores an entry from the verified private
// quarantine. Non-directories use an anchored hard link for the no-replace
// step. Directories use an anchored rename after checking that the destination
// is absent because filesystems do not permit hard links to directories.
func renameNoReplaceByLinkAt(
	oldDir *os.File,
	oldName string,
	newDir *os.File,
	newName string,
) error {
	return renameNoReplaceByLinkAtWithBeforeUnlink(oldDir, oldName, newDir, newName, nil)
}

func renameNoReplaceByLinkAtWithBeforeUnlink(
	oldDir *os.File,
	oldName string,
	newDir *os.File,
	newName string,
	beforeUnlink func() error,
) error {
	source, err := pathIdentityAt(oldDir, oldName)
	if err != nil {
		return err
	}
	if source.mode&unix.S_IFMT == unix.S_IFDIR {
		var destination unix.Stat_t
		if err := unix.Fstatat(int(newDir.Fd()), newName, &destination, unix.AT_SYMLINK_NOFOLLOW); err == nil {
			return os.ErrExist
		} else if !errors.Is(err, unix.ENOENT) {
			return err
		}
		return unix.Renameat(int(oldDir.Fd()), oldName, int(newDir.Fd()), newName)
	}

	if err := unix.Linkat(int(oldDir.Fd()), oldName, int(newDir.Fd()), newName, 0); err != nil {
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
	// Stat_t field widths vary across supported Unix targets.
	if uint64(oldStat.Dev) != source.dev || uint64(oldStat.Ino) != source.ino || //nolint:unconvert
		uint64(newStat.Dev) != source.dev || uint64(newStat.Ino) != source.ino { //nolint:unconvert
		return fmt.Errorf("quarantine or destination pathname changed during restore")
	}
	if beforeUnlink != nil {
		if err := beforeUnlink(); err != nil {
			return err
		}
	}
	oldAfter, err := pathIdentityAt(oldDir, oldName)
	if err != nil {
		return fmt.Errorf("inspect quarantine source before removal: %w", err)
	}
	newAfter, err := pathIdentityAt(newDir, newName)
	if err != nil {
		return fmt.Errorf("inspect restored path before source removal: %w", err)
	}
	if !samePathIdentity(source, oldAfter) || !samePathIdentity(source, newAfter) {
		return fmt.Errorf("quarantine or destination pathname changed before source removal")
	}
	return unix.Unlinkat(int(oldDir.Fd()), oldName, 0)
}

func removeAt(dir *os.File, name string, expected pathIdentity) error {
	current, err := pathIdentityAt(dir, name)
	if err != nil {
		return err
	}
	if !samePathIdentity(current, expected) {
		return fmt.Errorf("filesystem entry changed before removal")
	}
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
	created, err := pathIdentityAt(dir, name)
	if err != nil {
		return nil, err
	}

	privateDir, err := openDir(dir, name)
	if err != nil {
		_ = removeDirAt(dir, name, created)
		return nil, err
	}
	opened, err := pathIdentityForFile(privateDir)
	if err != nil {
		_ = privateDir.Close()
		_ = removeDirAt(dir, name, created)
		return nil, err
	}
	if !samePathIdentity(created, opened) {
		_ = privateDir.Close()
		_ = removeDirAt(dir, name, created)
		return nil, fmt.Errorf("quarantine directory changed between creation and open")
	}
	if err := verifyPrivateDir(privateDir); err != nil {
		_ = privateDir.Close()
		_ = removeDirAt(dir, name, created)
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

func removeDirAt(dir *os.File, name string, expected pathIdentity) error {
	current, err := pathIdentityAt(dir, name)
	if err != nil {
		return err
	}
	if !samePathIdentity(current, expected) {
		return fmt.Errorf("quarantine directory changed before removal")
	}
	return unix.Unlinkat(int(dir.Fd()), name, unix.AT_REMOVEDIR)
}
