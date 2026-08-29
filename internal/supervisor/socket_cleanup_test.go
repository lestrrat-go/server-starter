package supervisor

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRemoveExistingUnixSocketQuarantinesStaleSocket(t *testing.T) {
	requireSafeSocketQuarantine(t)
	path := filepath.Join(t.TempDir(), "listener.sock")
	makeStaleSocket(t, path)

	var quarantineEntry string
	err := removeSocketWithHooks(path, socketCleanupHooks{beforeRetain: func(path string) {
		quarantineEntry = path
	}})
	require.NoError(t, err)
	_, statErr := os.Lstat(path)
	require.ErrorIs(t, statErr, os.ErrNotExist)
	info, statErr := os.Lstat(quarantineEntry)
	require.NoError(t, statErr)
	require.NotZero(t, info.Mode()&os.ModeSocket)
}

func TestRemoveExistingUnixSocketAllowsSymlinkedParent(t *testing.T) {
	requireSafeSocketQuarantine(t)
	realParent := t.TempDir()
	symlinkParent := filepath.Join(t.TempDir(), "socket-parent")
	require.NoError(t, os.Symlink(realParent, symlinkParent))
	path := filepath.Join(symlinkParent, "listener.sock")

	require.NoError(t, removeExistingUnixSocket(path))
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), unixNetwork, path)
	require.NoError(t, err)
	require.NoError(t, listener.Close())
}

func TestRunStartsUnixListenerAtQuarantineBasename(t *testing.T) {
	requireSafeSocketQuarantine(t)
	path := filepath.Join(t.TempDir(), quarantineDirName)
	command, args := testWorkerCommand(t)
	starter, err := NewStarter(&config{
		command:   command,
		args:      args,
		paths:     []string{path},
		sigonterm: signalNameKill,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ctrl, err := starter.Run(ctx)
	require.NoError(t, err)
	info, err := os.Lstat(path)
	require.NoError(t, err)
	require.NotZero(t, info.Mode()&os.ModeSocket)

	cancel()
	require.ErrorIs(t, ctrl.Wait(), ErrServerClosed)
}

func TestRunStartsOverStaleUnixSocket(t *testing.T) {
	requireSafeSocketQuarantine(t)
	path := filepath.Join(t.TempDir(), "listener.sock")
	makeStaleSocket(t, path)
	command, args := testWorkerCommand(t)
	starter, err := NewStarter(&config{
		command:   command,
		args:      args,
		paths:     []string{path},
		sigonterm: signalNameKill,
	})
	require.NoError(t, err)

	// testing.T.Context requires Go 1.24, but this module supports Go 1.23.
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ctrl, err := starter.Run(ctx)
	require.NoError(t, err)
	info, err := os.Lstat(path)
	require.NoError(t, err)
	require.NotZero(t, info.Mode()&os.ModeSocket)

	cancel()
	require.ErrorIs(t, ctrl.Wait(), ErrServerClosed)
}

func TestRunReservesQuarantineBasenames(t *testing.T) {
	requireSafeSocketQuarantine(t)
	parent := t.TempDir()
	paths := []string{
		filepath.Join(parent, quarantineDirName),
		filepath.Join(parent, quarantineDirName+"-directory"),
	}
	command, args := testWorkerCommand(t)
	starter, err := NewStarter(&config{
		command:   command,
		args:      args,
		paths:     paths,
		sigonterm: signalNameKill,
	})
	require.NoError(t, err)

	// testing.T.Context requires Go 1.24, but this module supports Go 1.23.
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ctrl, err := starter.Run(ctx)
	require.NoError(t, err)
	for _, path := range paths {
		info, statErr := os.Lstat(path)
		require.NoError(t, statErr)
		require.NotZero(t, info.Mode()&os.ModeSocket)
	}

	cancel()
	require.ErrorIs(t, ctrl.Wait(), ErrServerClosed)
}

func TestRemoveExistingUnixSocketPreservesUnresolvedParentPath(t *testing.T) {
	requireSafeSocketQuarantine(t)
	parent := t.TempDir()
	victim := filepath.Join(parent, "victim.sock")
	makeStaleSocket(t, victim)
	path := parent + string(filepath.Separator) + "missing/../victim.sock"

	err := removeExistingUnixSocket(path)
	require.Error(t, err)
	info, statErr := os.Lstat(victim)
	require.NoError(t, statErr)
	require.NotZero(t, info.Mode()&os.ModeSocket)
}

func TestRemoveExistingUnixSocketRejectsNonSocket(t *testing.T) {
	requireSafeSocketQuarantine(t)
	for _, test := range []struct {
		name string
		make func(string) error
	}{
		{name: "file", make: func(path string) error { return os.WriteFile(path, []byte("keep"), 0o600) }},
		{name: "directory", make: func(path string) error { return os.Mkdir(path, 0o700) }},
		{name: "symlink", make: func(path string) error { return os.Symlink("target", path) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "listener")
			require.NoError(t, test.make(path))

			err := removeExistingUnixSocket(path)
			require.ErrorContains(t, err, "is not a socket")
			_, statErr := os.Lstat(path)
			require.NoError(t, statErr)
		})
	}
}

func TestRemoveExistingUnixSocketClassifiesQuarantinedEntry(t *testing.T) {
	requireSafeSocketQuarantine(t)
	path := filepath.Join(t.TempDir(), "listener.sock")
	makeStaleSocket(t, path)

	err := removeSocketWithHooks(path, socketCleanupHooks{beforeMove: func() {
		require.NoError(t, os.Remove(path))
		require.NoError(t, os.WriteFile(path, []byte("keep"), 0o600))
	}})
	require.ErrorContains(t, err, "is not a socket")
	contents, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	require.Equal(t, []byte("keep"), contents)
}

func TestRemoveExistingUnixSocketRetainsSocketThroughQuarantineHandle(t *testing.T) {
	requireSafeSocketQuarantine(t)
	path := filepath.Join(t.TempDir(), "listener.sock")
	makeStaleSocket(t, path)

	var movedDir string
	var retainedEntry string
	var replacementEntry string
	err := removeSocketWithHooks(path, socketCleanupHooks{beforeRetain: func(quarantineEntry string) {
		quarantineDir := filepath.Dir(quarantineEntry)
		movedDir = quarantineDir + ".moved"
		require.NoError(t, os.Rename(quarantineDir, movedDir))
		require.NoError(t, os.Mkdir(quarantineDir, 0o700))
		replacementEntry = quarantineEntry
		retainedEntry = filepath.Join(movedDir, filepath.Base(quarantineEntry))
		require.NoError(t, os.WriteFile(replacementEntry, []byte("keep"), 0o600))
	}})
	require.ErrorContains(t, err, "quarantine directory changed")
	contents, readErr := os.ReadFile(replacementEntry)
	require.NoError(t, readErr)
	require.Equal(t, []byte("keep"), contents)
	info, statErr := os.Lstat(retainedEntry)
	require.NoError(t, statErr)
	require.NotZero(t, info.Mode()&os.ModeSocket)
}

func TestRemoveExistingUnixSocketPreservesQuarantineEntryReplacement(t *testing.T) {
	requireSafeSocketQuarantine(t)
	path := filepath.Join(t.TempDir(), "listener.sock")
	makeStaleSocket(t, path)

	var replacementPath string
	err := removeSocketWithHooks(path, socketCleanupHooks{beforeRetain: func(quarantineEntry string) {
		replacementPath = quarantineEntry
		require.NoError(t, os.Remove(quarantineEntry))
		require.NoError(t, os.WriteFile(quarantineEntry, []byte("keep"), 0o600))
	}})
	require.ErrorContains(t, err, "changed before retention")
	contents, readErr := os.ReadFile(replacementPath)
	require.NoError(t, readErr)
	require.Equal(t, []byte("keep"), contents)
}

func TestRemoveExistingUnixSocketPreservesReplacementAfterIdentityCheck(t *testing.T) {
	requireSafeSocketQuarantine(t)
	path := filepath.Join(t.TempDir(), "listener.sock")
	makeStaleSocket(t, path)

	var retainedPath string
	var replacementPath string
	err := removeSocketWithHooks(path, socketCleanupHooks{afterRetentionIdentityCheck: func(quarantineEntry string) {
		retainedPath = quarantineEntry + ".retained"
		replacementPath = quarantineEntry
		require.NoError(t, os.Rename(quarantineEntry, retainedPath))
		require.NoError(t, os.WriteFile(replacementPath, []byte("keep"), 0o600))
	}})
	require.NoError(t, err)
	contents, readErr := os.ReadFile(replacementPath)
	require.NoError(t, readErr)
	require.Equal(t, []byte("keep"), contents)
	info, statErr := os.Lstat(retainedPath)
	require.NoError(t, statErr)
	require.NotZero(t, info.Mode()&os.ModeSocket)
}

func TestRemoveExistingUnixSocketUsesDistinctQuarantineSlots(t *testing.T) {
	requireSafeSocketQuarantine(t)
	parent := t.TempDir()
	firstPath := filepath.Join(parent, "first.sock")
	secondPath := filepath.Join(parent, "second.sock")
	makeStaleSocket(t, firstPath)
	makeStaleSocket(t, secondPath)

	firstReady := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstResult := make(chan error, 1)
	var firstSlot string
	go func() {
		firstResult <- removeSocketWithHooks(firstPath, socketCleanupHooks{beforeRetain: func(slot string) {
			firstSlot = slot
			close(firstReady)
			<-releaseFirst
		}})
	}()

	<-firstReady
	var secondSlot string
	secondErr := removeSocketWithHooks(secondPath, socketCleanupHooks{beforeRetain: func(slot string) {
		secondSlot = slot
	}})
	close(releaseFirst)
	firstErr := <-firstResult

	require.NotErrorIs(t, secondErr, os.ErrExist)
	require.NoError(t, secondErr)
	require.NoError(t, firstErr)
	require.NotEqual(t, firstSlot, secondSlot)
	for _, slot := range []string{firstSlot, secondSlot} {
		info, statErr := os.Lstat(slot)
		require.NoError(t, statErr)
		require.NotZero(t, info.Mode()&os.ModeSocket)
	}
	_, statErr := os.Lstat(secondPath)
	require.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestRemoveExistingUnixSocketUsesDistinctQuarantineSlotsForRepeatedPath(t *testing.T) {
	requireSafeSocketQuarantine(t)
	path := filepath.Join(t.TempDir(), "listener.sock")

	makeStaleSocket(t, path)
	var firstSlot string
	require.NoError(t, removeSocketWithHooks(path, socketCleanupHooks{beforeRetain: func(slot string) {
		firstSlot = slot
	}}))

	makeStaleSocket(t, path)
	var secondSlot string
	require.NoError(t, removeSocketWithHooks(path, socketCleanupHooks{beforeRetain: func(slot string) {
		secondSlot = slot
	}}))

	require.NotEqual(t, firstSlot, secondSlot)
	for _, slot := range []string{firstSlot, secondSlot} {
		info, statErr := os.Lstat(slot)
		require.NoError(t, statErr)
		require.NotZero(t, info.Mode()&os.ModeSocket)
	}
}

func TestRunAvoidsConfiguredQuarantineSlot(t *testing.T) {
	requireSafeSocketQuarantine(t)
	// Keep the parent short enough for the nested AF_UNIX listener path.
	//nolint:usetesting // t.TempDir paths exceed the AF_UNIX limit after quarantine components are added.
	parent, err := os.MkdirTemp("", "ss-")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(parent)) })
	path := filepath.Join(parent, "listener.sock")
	makeStaleSocket(t, path)

	quarantine, err := newSocketQuarantine(path, nil, socketCleanupHooks{})
	require.NoError(t, err)
	configuredSlot := quarantine.location()
	quarantine.close()
	makeStaleSocket(t, configuredSlot)

	command, args := testWorkerCommand(t)
	starter, err := NewStarter(&config{
		command:   command,
		args:      args,
		paths:     []string{path, configuredSlot},
		sigonterm: signalNameKill,
	})
	require.NoError(t, err)

	// testing.T.Context requires Go 1.24, but this module supports Go 1.23.
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ctrl, err := starter.Run(ctx)
	require.NoError(t, err)
	for _, listenerPath := range []string{path, configuredSlot} {
		info, statErr := os.Lstat(listenerPath)
		require.NoError(t, statErr)
		require.NotZero(t, info.Mode()&os.ModeSocket)
	}

	cancel()
	require.ErrorIs(t, ctrl.Wait(), ErrServerClosed)
}

func TestRemoveExistingUnixSocketRejectsQuarantineSubstitutionBeforeOpen(t *testing.T) {
	requireSafeSocketQuarantine(t)
	path := filepath.Join(t.TempDir(), "listener.sock")
	makeStaleSocket(t, path)

	err := removeSocketWithHooks(path, socketCleanupHooks{afterQuarantineMkdir: func(quarantineDir string) {
		require.NoError(t, os.Rename(quarantineDir, quarantineDir+".created"))
		require.NoError(t, os.Mkdir(quarantineDir, 0o777))
		require.NoError(t, os.Chmod(quarantineDir, 0o777))
	}})
	require.ErrorContains(t, err, "permissions")
	info, statErr := os.Lstat(path)
	require.NoError(t, statErr)
	require.NotZero(t, info.Mode()&os.ModeSocket)
}

func TestRemoveExistingUnixSocketRetainsReplacementAfterQuarantineOpenFailure(t *testing.T) {
	requireSafeSocketQuarantine(t)
	path := filepath.Join(t.TempDir(), "listener.sock")
	makeStaleSocket(t, path)
	var replacementDir string

	err := removeSocketWithHooks(path, socketCleanupHooks{
		afterQuarantineMkdir: func(quarantineDir string) {
			replacementDir = quarantineDir
			require.NoError(t, os.Remove(quarantineDir))
		},
		afterQuarantineOpenFailure: func(quarantineDir string) {
			require.Equal(t, replacementDir, quarantineDir)
			require.NoError(t, os.Mkdir(quarantineDir, 0o700))
		},
	})
	require.Error(t, err)
	info, statErr := os.Stat(replacementDir)
	require.NoError(t, statErr)
	require.True(t, info.IsDir())
}

func TestRemoveExistingUnixSocketDoesNotRecursivelyCleanReplacement(t *testing.T) {
	requireSafeSocketQuarantine(t)
	path := filepath.Join(t.TempDir(), "listener.sock")
	makeStaleSocket(t, path)

	var replacementKeep string
	err := removeSocketWithHooks(path, socketCleanupHooks{beforeCleanup: func(quarantineEntry string) {
		quarantineDir := filepath.Dir(quarantineEntry)
		require.NoError(t, os.Rename(quarantineDir, quarantineDir+".moved"))
		replacementKeep = filepath.Join(quarantineDir, "replacement", "keep")
		require.NoError(t, os.MkdirAll(filepath.Dir(replacementKeep), 0o700))
		require.NoError(t, os.WriteFile(replacementKeep, []byte("keep"), 0o600))
	}})
	require.ErrorContains(t, err, "quarantine directory changed")
	contents, readErr := os.ReadFile(replacementKeep)
	require.NoError(t, readErr)
	require.Equal(t, []byte("keep"), contents)
}

func TestRemoveExistingUnixSocketRetainsQuarantineDirectoryAfterCleanup(t *testing.T) {
	requireSafeSocketQuarantine(t)
	path := filepath.Join(t.TempDir(), "listener.sock")
	makeStaleSocket(t, path)

	var quarantineDir string
	err := removeSocketWithHooks(path, socketCleanupHooks{beforeCleanup: func(quarantineEntry string) {
		quarantineDir = filepath.Dir(quarantineEntry)
	}})
	require.NoError(t, err)
	info, statErr := os.Stat(quarantineDir)
	require.NoError(t, statErr)
	require.True(t, info.IsDir())
}

func TestRemoveExistingUnixSocketBoundsRetainedQuarantineDirectories(t *testing.T) {
	requireSafeSocketQuarantine(t)
	parent := t.TempDir()
	path := filepath.Join(parent, "listener.sock")

	for range 5 {
		require.NoError(t, removeExistingUnixSocket(path))
	}

	entries, err := os.ReadDir(parent)
	require.NoError(t, err)
	var quarantineCount int
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), quarantineDirPrefix) {
			quarantineCount++
		}
	}
	require.Equal(t, 1, quarantineCount)
}

func TestRemoveExistingUnixSocketRetainsEmptyCleanupReplacement(t *testing.T) {
	requireSafeSocketQuarantine(t)
	path := filepath.Join(t.TempDir(), "listener.sock")
	makeStaleSocket(t, path)

	var movedDir string
	var replacementDir string
	err := removeSocketWithHooks(path, socketCleanupHooks{beforeCleanup: func(quarantineEntry string) {
		replacementDir = filepath.Dir(quarantineEntry)
		movedDir = replacementDir + ".moved"
		require.NoError(t, os.Rename(replacementDir, movedDir))
		require.NoError(t, os.Mkdir(replacementDir, 0o700))
	}})
	require.ErrorContains(t, err, "quarantine directory changed")
	for _, dir := range []string{movedDir, replacementDir} {
		info, statErr := os.Stat(dir)
		require.NoError(t, statErr)
		require.True(t, info.IsDir())
	}
}

func TestRemoveExistingUnixSocketFailsClosedWhenSafeQuarantineIsUnavailable(t *testing.T) {
	if safeSocketQuarantineAvailable() {
		t.Skip("safe quarantine is available on this platform")
	}
	path := filepath.Join(t.TempDir(), "listener")
	require.NoError(t, os.WriteFile(path, []byte("keep"), 0o600))

	err := removeExistingUnixSocket(path)
	require.ErrorContains(t, err, "identity-safe unix socket cleanup is unavailable")
	contents, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	require.Equal(t, []byte("keep"), contents)
	require.NoError(t, removeExistingUnixSocket(filepath.Join(t.TempDir(), "missing")))
}

func makeStaleSocket(t *testing.T, path string) {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), unixNetwork, path)
	require.NoError(t, err)
	backup := path + ".stale"
	require.NoError(t, os.Rename(path, backup))
	require.NoError(t, listener.Close())
	require.NoError(t, os.Rename(backup, path))
}

func TestRemoveExistingUnixSocketSkipsLinuxAbstractAddress(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux abstract addresses are Linux-only")
	}
	require.NoError(t, removeExistingUnixSocket("@server-starter-test"))
}

func requireSafeSocketQuarantine(t *testing.T) {
	t.Helper()
	if !safeSocketQuarantineAvailable() {
		t.Skip("identity-safe socket quarantine is unavailable on this platform")
	}
}
