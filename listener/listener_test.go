package listener

import (
	"os"
	"testing"
)

func TestPort(t *testing.T) {
	expect := List{
		TCPListener{Addr: "127.0.0.1", Port: 9090, fd: 4},
		TCPListener{Addr: "0.0.0.0", Port: 8080, fd: 5},
		UnixListener{Path: "/foo/bar/baz.sock", fd: 6},
	}

	os.Setenv("SERVER_STARTER_PORT", expect.String())
	ports, err := Ports()
	if err != nil {
		t.Errorf("Failed to parse ports from env: %s", err)
	}

	for i, port := range ports {
		if port.Fd() != expect[i].Fd() {
			t.Errorf("parsed fd is not what we expected (expected %d, got %d)", expect[i].Fd(), port.Fd())
		}
		_, gotTcp := port.(TCPListener)
		_, expectTcp := expect[i].(TCPListener)
		if gotTcp != expectTcp {
			t.Errorf("parsed listener is the wrong type")
		}
	}
}

func TestPortNoEnv(t *testing.T) {
	os.Setenv("SERVER_STARTER_PORT", "")

	ports, err := Ports()
	if err != ErrNoListeningTarget {
		t.Error("Ports must return error if no env")
	}

	if ports != nil {
		t.Errorf("Ports must return nil if no env")
	}
}
