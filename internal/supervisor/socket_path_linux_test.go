package supervisor

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	starter "github.com/lestrrat-go/server-starter/v2"
	"github.com/stretchr/testify/require"
)

func TestRunSupportsLinuxNonFilesystemUnixAddresses(t *testing.T) {
	name := fmt.Sprintf("server-starter-%d-%d", os.Getpid(), time.Now().UnixNano())
	for _, test := range []struct {
		name      string
		path      string
		wantPath  string
		autobound bool
	}{
		{name: "autobind", autobound: true},
		{name: "at-prefixed abstract", path: "@" + name + "-at", wantPath: "@" + name + "-at"},
		{name: "NUL-prefixed abstract", path: "\x00" + name + "-nul", wantPath: "@" + name + "-nul"},
	} {
		t.Run(test.name, func(t *testing.T) {
			reportPath := filepath.Join(t.TempDir(), "ports")
			sd, err := NewStarter(&config{
				command:   testShellPath,
				args:      []string{"-c", `printf '%s\n' "$SERVER_STARTER_PORT" > "$1"; exec sleep 30`, "worker", reportPath},
				paths:     []string{test.path},
				sigonterm: "KILL",
				stderr:    io.Discard,
			})
			require.NoError(t, err)

			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			ctrl, err := sd.Run(ctx)
			require.NoError(t, err)

			portSpec := strings.TrimSpace(string(readFileEventually(t, reportPath)))
			listeners, err := starter.ParsePorts(portSpec)
			require.NoError(t, err)
			require.Len(t, listeners, 1)
			unixListener, ok := listeners[0].(starter.UnixListener)
			require.True(t, ok)
			if test.autobound {
				require.True(t, strings.HasPrefix(unixListener.Path, "@"))
			} else {
				require.Equal(t, test.wantPath, unixListener.Path)
			}
			require.NotContains(t, portSpec, "\x00")

			conn, err := net.DialTimeout(unixNetwork, unixListener.Path, time.Second)
			require.NoError(t, err)
			require.NoError(t, conn.Close())

			cancel()
			require.ErrorIs(t, ctrl.Wait(), ErrServerClosed)
		})
	}
}
