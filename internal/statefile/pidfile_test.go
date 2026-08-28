package statefile

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAcquirePIDFileWritesNewlineAndRemovesOwnedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.pid")
	pid, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(data), "\n") {
		t.Fatalf("pid file %q has no trailing newline", data)
	}
	if err := pid.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("pid file still exists, stat error = %v", err)
	}
}

func TestAcquirePIDFileReportsExistingOwnerWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.pid")
	owner, err := Acquire(path)
	require.NoError(t, err)
	defer owner.Close()

	result := make(chan error, 1)
	go func() {
		contender, err := Acquire(path)
		if contender != nil {
			_ = contender.Close()
		}
		result <- err
	}()

	select {
	case err := <-result:
		require.Error(t, err)
		require.ErrorContains(t, err, path)
		require.ErrorContains(t, err, strconv.Itoa(os.Getpid()))
	case <-time.After(time.Second):
		require.NoError(t, owner.Close())
		<-result
		t.Fatal("second Acquire blocked on the occupied pid file")
	}

	require.NoError(t, owner.Close())
	replacement, err := Acquire(path)
	require.NoError(t, err)
	require.NoError(t, replacement.Close())
}
