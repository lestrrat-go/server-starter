//go:build linux

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
	beforePublish                      func(string) error
	afterPublish                       func(string) error
	afterPrivateDirectoryIdentityCheck func(string)
}

var errSocketPublicationSourceChanged = errors.New("private unix socket changed before publication")

func listenFilesystemUnixSocket(
	ctx context.Context,
	path string,
	hooks socketPublicationHooks,
) (net.Listener, *socketCleanupState, error) {
	parentPath, _ := filepath.Split(path)
	if parentPath == "" {
		parentPath = "." + string(filepath.Separator)
	}
	parentFD, err := openQuarantineDirectory(parentPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open unix socket parent directory for %q: %w", path, err)
	}
	keepParent := false
	defer func() {
		if !keepParent {
			_ = unix.Close(parentFD)
		}
	}()

	privateDir, err := os.MkdirTemp(parentPath, ".")
	if err != nil {
		return nil, nil, fmt.Errorf("create private unix socket directory for %q: %w", path, err)
	}
	privateName := filepath.Base(privateDir)
	privateFD, err := openQuarantineDirectoryAt(parentFD, privateName)
	if err != nil {
		return nil, nil, fmt.Errorf("open private unix socket directory for %q: %w", path, err)
	}
	defer unix.Close(privateFD)

	var privateDirStat unix.Stat_t
	if err := unix.Fstat(privateFD, &privateDirStat); err != nil {
		return nil, nil, fmt.Errorf("inspect private unix socket directory for %q: %w", path, err)
	}
	if err := validateQuarantineDirectory(&privateDirStat); err != nil {
		return nil, nil, fmt.Errorf("validate private unix socket directory for %q: %w", path, err)
	}
	defer retainPrivateSocketDirectory(parentFD, privateName, &privateDirStat, hooks)
	const privateSocketName = "s"

	lc := listenConfig(unixNetwork)
	l, err := lc.Listen(ctx, unixNetwork, privateSocketBindPath(privateFD, privateSocketName))
	if err != nil {
		return nil, nil, err
	}
	keepListener := false
	defer func() {
		if !keepListener {
			_ = l.Close()
		}
	}()

	unixListener, ok := l.(*net.UnixListener)
	if !ok {
		return nil, nil, fmt.Errorf(
			"listen private unix socket for %q returned %T, want *net.UnixListener",
			path,
			l,
		)
	}
	unixListener.SetUnlinkOnClose(false)

	sourceFD, err := openQuarantineSource(privateFD, privateSocketName)
	if err != nil {
		return nil, nil, fmt.Errorf("retain bound unix socket for %q: %w", path, err)
	}
	defer unix.Close(sourceFD)
	var sourceStat unix.Stat_t
	if err := unix.Fstat(sourceFD, &sourceStat); err != nil {
		return nil, nil, fmt.Errorf("capture bound unix socket identity for %q: %w", path, err)
	}
	if sourceStat.Mode&unix.S_IFMT != unix.S_IFSOCK {
		return nil, nil, errSocketPublicationSourceChanged
	}
	identity := socketIdentityFromStat(&sourceStat)
	if hooks.beforePublish != nil {
		if err := hooks.beforePublish(path); err != nil {
			return nil, nil, fmt.Errorf("before publishing unix socket %q: %w", path, err)
		}
	}
	_, publicName := filepath.Split(path)
	if err := publishUnixSocketNoReplace(sourceFD, parentFD, publicName, identity); err != nil {
		return nil, nil, fmt.Errorf("publish unix socket %q: %w", path, err)
	}
	cleanup := &socketCleanupState{
		parentFD:   parentFD,
		parentPath: parentPath,
		publicName: publicName,
		identity:   identity,
	}
	if hooks.afterPublish != nil {
		if err := hooks.afterPublish(path); err != nil {
			cleanupErr := cleanup.remove(configuredSocketPaths([]string{path}))
			return nil, nil, errors.Join(
				fmt.Errorf("after publishing unix socket %q: %w", path, err),
				cleanupErr,
			)
		}
	}

	keepListener = true
	keepParent = true
	return l, cleanup, nil
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
	sourceFD int,
	publicFD int,
	publicName string,
	identity socketIdentity,
) error {
	if err := linkSocketFDNoReplace(sourceFD, publicFD, publicName); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return errSocketPublicationSourceChanged
		}
		return err
	}
	publishedIdentity, err := socketIdentityAt(publicFD, publicName)
	if err != nil {
		return err
	}
	if publishedIdentity != identity {
		return errSocketPublicationSourceChanged
	}
	return nil
}

func retainPrivateSocketDirectory(
	parentFD int,
	name string,
	retainedStat *unix.Stat_t,
	hooks socketPublicationHooks,
) {
	var currentStat unix.Stat_t
	if err := unix.Fstatat(parentFD, name, &currentStat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return
	}
	if !sameUnixIdentity(retainedStat, &currentStat) || currentStat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return
	}
	if hooks.afterPrivateDirectoryIdentityCheck != nil {
		hooks.afterPrivateDirectoryIdentityCheck(name)
	}
	// Removing by name after the identity check could delete a replacement.
	// Retain the directory instead.
}

func socketCleanupStateForPath(path string, identity socketIdentity) (*socketCleanupState, error) {
	parentPath, publicName := filepath.Split(path)
	if parentPath == "" {
		parentPath = "." + string(filepath.Separator)
	}
	parentFD, err := openQuarantineDirectory(parentPath)
	if err != nil {
		return nil, err
	}
	return &socketCleanupState{
		parentFD:   parentFD,
		parentPath: parentPath,
		publicName: publicName,
		identity:   identity,
	}, nil
}

func (s *socketCleanupState) remove(configuredPaths *configuredSocketPathSet) error {
	parentFD, err := unix.Dup(s.parentFD)
	if err != nil {
		return fmt.Errorf("duplicate unix socket parent directory descriptor: %w", err)
	}
	path := s.parentPath + s.publicName
	return removeOwnedUnixSocketAt(
		parentFD,
		s.parentPath,
		s.publicName,
		path,
		configuredPaths,
		s.identity,
	)
}

func (s *socketCleanupState) close() {
	_ = unix.Close(s.parentFD)
}
