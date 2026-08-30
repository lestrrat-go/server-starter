//go:build linux || darwin

package supervisor

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

type socketPublicationHooks struct {
	beforePublish func(string) error
	afterPublish  func(string) error
}

var errSocketPublicationSourceChanged = errors.New("private unix socket changed before publication")

func listenFilesystemUnixSocket(
	ctx context.Context,
	path string,
	hooks socketPublicationHooks,
) (net.Listener, socketIdentity, error) {
	parentPath, _ := filepath.Split(path)
	if parentPath == "" {
		parentPath = "." + string(filepath.Separator)
	}
	parentFD, err := openQuarantineDirectory(parentPath)
	if err != nil {
		return nil, socketIdentity{}, fmt.Errorf("open unix socket parent directory for %q: %w", path, err)
	}
	defer unix.Close(parentFD)

	privateDir, err := os.MkdirTemp(parentPath, ".")
	if err != nil {
		return nil, socketIdentity{}, fmt.Errorf("create private unix socket directory for %q: %w", path, err)
	}
	privateName := filepath.Base(privateDir)
	privateFD, err := openQuarantineDirectoryAt(parentFD, privateName)
	if err != nil {
		return nil, socketIdentity{}, fmt.Errorf("open private unix socket directory for %q: %w", path, err)
	}
	defer unix.Close(privateFD)

	var privateDirStat unix.Stat_t
	if err := unix.Fstat(privateFD, &privateDirStat); err != nil {
		return nil, socketIdentity{}, fmt.Errorf("inspect private unix socket directory for %q: %w", path, err)
	}
	if err := validateQuarantineDirectory(&privateDirStat); err != nil {
		return nil, socketIdentity{}, fmt.Errorf("validate private unix socket directory for %q: %w", path, err)
	}
	const privateSocketName = "s"
	defer func() {
		_ = unix.Unlinkat(privateFD, privateSocketName, 0)
		_ = removeRetainedPrivateSocketDirectory(parentFD, privateName, &privateDirStat)
	}()

	lc := listenConfig(unixNetwork)
	l, err := lc.Listen(ctx, unixNetwork, privateSocketBindPath(privateFD, privateSocketName))
	if err != nil {
		return nil, socketIdentity{}, err
	}
	keepListener := false
	defer func() {
		if !keepListener {
			_ = l.Close()
		}
	}()

	unixListener, ok := l.(*net.UnixListener)
	if !ok {
		return nil, socketIdentity{}, fmt.Errorf(
			"listen private unix socket for %q returned %T, want *net.UnixListener",
			path,
			l,
		)
	}
	unixListener.SetUnlinkOnClose(false)

	identity, err := socketIdentityAt(privateFD, privateSocketName)
	if err != nil {
		return nil, socketIdentity{}, fmt.Errorf("capture bound unix socket identity for %q: %w", path, err)
	}
	if hooks.beforePublish != nil {
		if err := hooks.beforePublish(path); err != nil {
			return nil, socketIdentity{}, fmt.Errorf("before publishing unix socket %q: %w", path, err)
		}
	}
	_, publicName := filepath.Split(path)
	if err := publishUnixSocketNoReplace(privateFD, privateSocketName, parentFD, publicName, identity); err != nil {
		return nil, socketIdentity{}, fmt.Errorf("publish unix socket %q: %w", path, err)
	}
	if hooks.afterPublish != nil {
		if err := hooks.afterPublish(path); err != nil {
			cleanupErr := removeOwnedUnixSocket(path, configuredSocketPaths([]string{path}), identity)
			return nil, socketIdentity{}, errors.Join(
				fmt.Errorf("after publishing unix socket %q: %w", path, err),
				cleanupErr,
			)
		}
	}

	keepListener = true
	return l, identity, nil
}

func socketIdentityAt(parentFD int, name string) (socketIdentity, error) {
	var stat unix.Stat_t
	if err := unix.Fstatat(parentFD, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return socketIdentity{}, err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFSOCK {
		return socketIdentity{}, errSocketPublicationSourceChanged
	}
	return socketIdentityFromStat(&stat), nil
}

func publishUnixSocketNoReplace(
	privateFD int,
	privateName string,
	publicFD int,
	publicName string,
	identity socketIdentity,
) error {
	currentIdentity, err := socketIdentityAt(privateFD, privateName)
	if err != nil {
		return err
	}
	if currentIdentity != identity {
		return errSocketPublicationSourceChanged
	}
	return renameSocketEntryNoReplace(privateFD, privateName, publicFD, publicName)
}

func removeRetainedPrivateSocketDirectory(parentFD int, name string, retainedStat *unix.Stat_t) error {
	var currentStat unix.Stat_t
	if err := unix.Fstatat(parentFD, name, &currentStat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if !sameUnixIdentity(retainedStat, &currentStat) || currentStat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return errSocketSourceChanged
	}
	return unix.Unlinkat(parentFD, name, unix.AT_REMOVEDIR)
}
