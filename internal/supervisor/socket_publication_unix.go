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

func listenFilesystemUnixSocket(
	ctx context.Context,
	path string,
	hooks socketPublicationHooks,
) (net.Listener, socketIdentity, error) {
	parentPath, _ := filepath.Split(path)
	if parentPath == "" {
		parentPath = "." + string(filepath.Separator)
	}
	privateDir, err := os.MkdirTemp(parentPath, ".")
	if err != nil {
		return nil, socketIdentity{}, fmt.Errorf("create private unix socket directory for %q: %w", path, err)
	}
	privatePath := filepath.Join(privateDir, "s")
	defer func() {
		_ = os.Remove(privatePath)
		_ = os.Remove(privateDir)
	}()

	lc := listenConfig(unixNetwork)
	l, err := lc.Listen(ctx, unixNetwork, privatePath)
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

	identity, err := socketIdentityForPath(privatePath)
	if err != nil {
		return nil, socketIdentity{}, fmt.Errorf("capture bound unix socket identity for %q: %w", path, err)
	}
	if hooks.beforePublish != nil {
		if err := hooks.beforePublish(path); err != nil {
			return nil, socketIdentity{}, fmt.Errorf("before publishing unix socket %q: %w", path, err)
		}
	}
	if err := renameUnixSocketNoReplace(privatePath, path); err != nil {
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

func renameUnixSocketNoReplace(oldPath, newPath string) error {
	oldParentPath, oldName := filepath.Split(oldPath)
	if oldParentPath == "" {
		oldParentPath = "." + string(filepath.Separator)
	}
	newParentPath, newName := filepath.Split(newPath)
	if newParentPath == "" {
		newParentPath = "." + string(filepath.Separator)
	}

	oldParentFD, err := openQuarantineDirectory(oldParentPath)
	if err != nil {
		return err
	}
	defer unix.Close(oldParentFD)
	newParentFD, err := openQuarantineDirectory(newParentPath)
	if err != nil {
		return err
	}
	defer unix.Close(newParentFD)

	return renameSocketEntryNoReplace(oldParentFD, oldName, newParentFD, newName)
}
