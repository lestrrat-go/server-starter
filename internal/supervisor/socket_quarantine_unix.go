//go:build linux || darwin

package supervisor

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

type unixSocketQuarantine struct {
	parentFD   int
	dirFD      int
	parentPath string
	dirName    string
	sourceName string
	slotName   string
	dirStat    unix.Stat_t
	sourceFD   int
	sourceStat unix.Stat_t
	hooks      socketCleanupHooks
}

func newSocketQuarantine(
	path string,
	configuredPaths *configuredSocketPathSet,
	hooks socketCleanupHooks,
) (socketQuarantine, error) {
	parentPath, sourceName := filepath.Split(path)
	if parentPath == "" {
		parentPath = "." + string(filepath.Separator)
	}
	parentFD, err := openQuarantineDirectory(parentPath)
	if err != nil {
		return nil, err
	}
	var parentStat unix.Stat_t
	if err := unix.Fstat(parentFD, &parentStat); err != nil {
		_ = unix.Close(parentFD)
		return nil, err
	}
	parentIdentity := socketDirectoryIdentityFromStat(&parentStat)
	sourceFD, sourceErr := openQuarantineSource(parentFD, sourceName)
	if sourceErr != nil && !os.IsNotExist(sourceErr) {
		_ = unix.Close(parentFD)
		return nil, sourceErr
	}
	var sourceStat unix.Stat_t
	if sourceFD >= 0 {
		if err := unix.Fstat(sourceFD, &sourceStat); err != nil {
			_ = unix.Close(sourceFD)
			_ = unix.Close(parentFD)
			return nil, err
		}
	}

	dirName := socketQuarantineDirectoryName(parentPath, parentIdentity, sourceName, configuredPaths)
	created, createdStat, err := ensureQuarantineDir(parentFD, dirName)
	if err != nil {
		if sourceFD >= 0 {
			_ = unix.Close(sourceFD)
		}
		_ = unix.Close(parentFD)
		return nil, err
	}
	dirPath := parentPath + dirName
	if created && hooks.afterQuarantineMkdir != nil {
		hooks.afterQuarantineMkdir(dirPath)
	}
	dirFD, err := openQuarantineDirectoryAt(parentFD, dirName)
	if err != nil {
		if hooks.afterQuarantineOpenFailure != nil {
			hooks.afterQuarantineOpenFailure(dirPath)
		}
		if sourceFD >= 0 {
			_ = unix.Close(sourceFD)
		}
		_ = unix.Close(parentFD)
		return nil, err
	}

	quarantine := &unixSocketQuarantine{
		parentFD:   parentFD,
		dirFD:      dirFD,
		parentPath: parentPath,
		dirName:    dirName,
		sourceName: sourceName,
		sourceFD:   sourceFD,
		sourceStat: sourceStat,
		hooks:      hooks,
	}
	if err := unix.Fstat(dirFD, &quarantine.dirStat); err != nil {
		quarantine.close()
		return nil, err
	}
	if err := validateQuarantineDirectory(&quarantine.dirStat); err != nil {
		quarantine.close()
		return nil, err
	}
	if created && !sameUnixIdentity(&createdStat, &quarantine.dirStat) {
		quarantine.close()
		return nil, fmt.Errorf("quarantine directory changed while opening it")
	}
	quarantine.slotName = socketQuarantineSlotName(
		parentPath,
		dirName,
		socketDirectoryIdentityFromStat(&quarantine.dirStat),
		sourceName,
		&sourceStat,
		configuredPaths,
	)
	return quarantine, nil
}

func socketDirectoryIdentityForPath(path string) (socketDirectoryIdentity, error) {
	var stat unix.Stat_t
	if err := unix.Stat(path, &stat); err != nil {
		return socketDirectoryIdentity{}, err
	}
	return socketDirectoryIdentityFromStat(&stat), nil
}

func socketIdentityForPath(path string) (socketIdentity, error) {
	var stat unix.Stat_t
	if err := unix.Lstat(path, &stat); err != nil {
		return socketIdentity{}, err
	}
	return socketIdentityFromStat(&stat), nil
}

func socketIdentityFromStat(stat *unix.Stat_t) socketIdentity {
	//nolint:unconvert // Darwin represents Stat_t.Dev as int32.
	return socketIdentity{device: uint64(stat.Dev), inode: stat.Ino}
}

func socketDirectoryIdentityFromStat(stat *unix.Stat_t) socketDirectoryIdentity {
	//nolint:unconvert // Darwin represents Stat_t.Dev as int32.
	return socketDirectoryIdentity{device: uint64(stat.Dev), inode: stat.Ino}
}

func ensureQuarantineDir(parentFD int, dirName string) (bool, unix.Stat_t, error) {
	var stat unix.Stat_t
	if err := unix.Mkdirat(parentFD, dirName, 0o700); err != nil {
		if err == unix.EEXIST {
			if err := unix.Fstatat(parentFD, dirName, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
				return false, stat, err
			}
			return false, stat, nil
		}
		return false, stat, err
	}
	if err := unix.Fstatat(parentFD, dirName, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return false, stat, err
	}
	return true, stat, nil
}

func validateQuarantineDirectory(stat *unix.Stat_t) error {
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return fmt.Errorf("quarantine path is not a directory")
	}
	if stat.Uid != uint32(os.Geteuid()) {
		return fmt.Errorf("quarantine directory owner %d does not match effective user %d", stat.Uid, os.Geteuid())
	}
	permissions := stat.Mode & 0o777
	if permissions&0o077 != 0 || permissions&0o300 != 0o300 {
		return fmt.Errorf("quarantine directory permissions %#o are not private and write-search capable", permissions)
	}
	return nil
}

func (q *unixSocketQuarantine) moveIn() error {
	if q.sourceFD < 0 {
		return errSocketSourceUnavailable
	}
	return renameSocketEntryNoReplace(q.parentFD, q.sourceName, q.dirFD, q.slotName)
}

func (q *unixSocketQuarantine) entryIsSocket() (bool, error) {
	var stat unix.Stat_t
	if err := unix.Fstatat(q.dirFD, q.slotName, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return false, err
	}
	if !sameUnixIdentity(&q.sourceStat, &stat) {
		return false, errSocketSourceChanged
	}
	return stat.Mode&unix.S_IFMT == unix.S_IFSOCK, nil
}

func (q *unixSocketQuarantine) entryMatchesIdentity(identity socketIdentity) (bool, error) {
	var stat unix.Stat_t
	if err := unix.Fstatat(q.dirFD, q.slotName, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return false, err
	}
	if !sameUnixIdentity(&q.sourceStat, &stat) {
		return false, errSocketSourceChanged
	}
	return socketIdentityFromStat(&stat) == identity, nil
}

func (q *unixSocketQuarantine) restore() error {
	var stat unix.Stat_t
	if err := unix.Fstatat(q.dirFD, q.slotName, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if !sameUnixIdentity(&q.sourceStat, &stat) {
		return fmt.Errorf("quarantined unix socket changed before restore")
	}
	return renameSocketEntryNoReplace(q.dirFD, q.slotName, q.parentFD, q.sourceName)
}

func (q *unixSocketQuarantine) removeEntry() error {
	var stat unix.Stat_t
	if err := unix.Fstatat(q.dirFD, q.slotName, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if !sameUnixIdentity(&q.sourceStat, &stat) {
		return fmt.Errorf("quarantined unix socket changed before removal")
	}
	return unix.Unlinkat(q.dirFD, q.slotName, 0)
}

func (q *unixSocketQuarantine) retainEntry() error {
	var stat unix.Stat_t
	if err := unix.Fstatat(q.dirFD, q.slotName, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if !sameUnixIdentity(&q.sourceStat, &stat) {
		return fmt.Errorf("quarantined unix socket changed before retention")
	}
	if q.hooks.afterRetentionIdentityCheck != nil {
		q.hooks.afterRetentionIdentityCheck(q.location())
	}
	// Unix unlink APIs cannot require the directory entry to match an expected
	// device and inode. The original listener path is already free after the
	// move, so retain the socket in quarantine and let startup continue.
	return nil
}

func (q *unixSocketQuarantine) cleanup() error {
	var current unix.Stat_t
	if err := unix.Fstatat(q.parentFD, q.dirName, &current, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if current.Dev != q.dirStat.Dev || current.Ino != q.dirStat.Ino || current.Mode&unix.S_IFMT != unix.S_IFDIR {
		return fmt.Errorf("quarantine directory changed; original directory retained through its open handle")
	}
	// Removing the directory by name would resolve it again after the identity
	// check and could remove a replacement. Retain the empty directory instead.
	return nil
}

func (q *unixSocketQuarantine) close() {
	_ = unix.Close(q.dirFD)
	_ = unix.Close(q.parentFD)
	if q.sourceFD >= 0 {
		_ = unix.Close(q.sourceFD)
	}
}

func (q *unixSocketQuarantine) location() string {
	return q.parentPath + q.dirName + string(filepath.Separator) + q.slotName
}

func safeSocketQuarantineAvailable() bool {
	return true
}

func sameUnixIdentity(a, b *unix.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino
}

func socketQuarantineSlotName(
	parentPath string,
	dirName string,
	parentIdentity socketDirectoryIdentity,
	sourceName string,
	sourceStat *unix.Stat_t,
	configuredPaths *configuredSocketPathSet,
) string {
	identity := fmt.Sprintf("%s\x00%d\x00%d", sourceName, sourceStat.Dev, sourceStat.Ino)
	digest := sha256.Sum256([]byte(identity))
	baseName := quarantineEntryPrefix + base64.RawURLEncoding.EncodeToString(digest[:])
	for suffix := 0; ; suffix++ {
		name := baseName
		if suffix > 0 {
			name += fmt.Sprintf("-%d", suffix)
		}
		path := parentPath + dirName + string(filepath.Separator) + name
		if !configuredPaths.contains(parentIdentity, name, path) {
			return name
		}
	}
}

func socketQuarantineDirectoryName(
	parentPath string,
	parentIdentity socketDirectoryIdentity,
	sourceName string,
	configuredPaths *configuredSocketPathSet,
) string {
	for suffix := 0; ; suffix++ {
		name := quarantineDirName
		if suffix == 1 {
			name += "-directory"
		} else if suffix > 1 {
			name += fmt.Sprintf("-directory-%d", suffix-1)
		}
		if name == sourceName {
			continue
		}
		path := parentPath + name
		if configuredPaths.contains(parentIdentity, name, path) {
			continue
		}
		return name
	}
}
