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

type pathIdentity struct {
	dev           uint64
	ino           uint64
	mode          uint32
	creationToken string
}

func pathIdentityFromStat(stat *unix.Stat_t) pathIdentity {
	// Stat_t field widths vary across supported Unix targets.
	return pathIdentity{
		dev:  uint64(stat.Dev),  //nolint:unconvert
		ino:  uint64(stat.Ino),  //nolint:unconvert
		mode: uint32(stat.Mode), //nolint:unconvert
	}
}

func pathIdentityAt(dir *os.File, name string) (pathIdentity, error) {
	var stat unix.Stat_t
	if err := unix.Fstatat(int(dir.Fd()), name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return pathIdentity{}, err
	}
	identity := pathIdentityFromStat(&stat)
	identity.creationToken = creationTokenAt(dir, name, &stat)
	return identity, nil
}

func pathIdentityForFile(file *os.File) (pathIdentity, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &stat); err != nil {
		return pathIdentity{}, err
	}
	identity := pathIdentityFromStat(&stat)
	identity.creationToken = creationTokenForFile(file, &stat)
	return identity, nil
}

func samePathIdentity(left, right pathIdentity) bool {
	return left.dev == right.dev && left.ino == right.ino &&
		left.mode&unix.S_IFMT == right.mode&unix.S_IFMT
}

func sameCreationIdentity(left, right pathIdentity) bool {
	if !samePathIdentity(left, right) {
		return false
	}
	return left.creationToken != "" && left.creationToken == right.creationToken
}

func (identity pathIdentity) isSocket() bool {
	return identity.mode&unix.S_IFMT == unix.S_IFSOCK
}

func pinSocketAt(oldDir *os.File, oldName string, pinDir *os.File, pinName string) (pathIdentity, error) {
	if err := linkAt(oldDir, oldName, pinDir, pinName); err != nil {
		return pathIdentity{}, err
	}
	pinned, err := pathIdentityAt(pinDir, pinName)
	if err != nil {
		_ = unix.Unlinkat(int(pinDir.Fd()), pinName, 0)
		return pathIdentity{}, err
	}
	return pinned, nil
}

func unpinSocketAt(pinDir *os.File, pinName string, expected pathIdentity) error {
	return removeAt(pinDir, pinName, expected)
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
// step. Directories stay quarantined because filesystems do not permit hard
// links to directories and these targets have no atomic no-replace rename.
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
		return errRenameNoReplaceUnsupported
	}

	if err := linkAt(oldDir, oldName, newDir, newName); err != nil {
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
	if !sameCreationIdentity(created, opened) {
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
	if !sameCreationIdentity(current, expected) {
		return fmt.Errorf("quarantine directory changed before removal")
	}
	return unix.Unlinkat(int(dir.Fd()), name, unix.AT_REMOVEDIR)
}

func closeAndRemoveSocketQuarantine(parent, quarantine *os.File, name string) error {
	opened, err := pathIdentityForFile(quarantine)
	if err != nil {
		_ = quarantine.Close()
		return err
	}
	current, err := pathIdentityAt(parent, name)
	if err != nil {
		_ = quarantine.Close()
		return err
	}
	if !sameCreationIdentity(opened, current) {
		_ = quarantine.Close()
		return fmt.Errorf("quarantine directory changed before removal")
	}
	if err := unix.Unlinkat(int(parent.Fd()), name, unix.AT_REMOVEDIR); err != nil {
		_ = quarantine.Close()
		return err
	}
	return quarantine.Close()
}
