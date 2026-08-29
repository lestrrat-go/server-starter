package supervisor

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParsePortTarget(t *testing.T) {
	for _, test := range []struct {
		name, spec, network, host string
		port, fd                  int
	}{
		{name: "tcp4", spec: "8080", network: "tcp4", port: 8080, fd: -1},
		{name: "tcp4 hostname beginning with u", spec: "ubuntu.internal:8080", network: "tcp4", host: "ubuntu.internal", port: 8080, fd: -1},
		{name: "udp4", spec: "udp://8080", network: "udp4", port: 8080, fd: -1},
		{name: "legacy udp4", spec: "u8080", network: "udp4", port: 8080, fd: -1},
		{
			name: "legacy udp4 hostname beginning with u", spec: "ubuntu.internal:u8080",
			network: "udp4", host: "ubuntu.internal", port: 8080, fd: -1,
		},
		{name: "tcp6", spec: "[::1]:8080", network: "tcp6", host: "::1", port: 8080, fd: -1},
		{name: "udp6", spec: "udp://[::1]:8080=7", network: "udp6", host: "::1", port: 8080, fd: 7},
		{name: "legacy udp6", spec: "u[::1]:8080=7", network: "udp6", host: "::1", port: 8080, fd: 7},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := parsePortTarget(test.spec)
			if err != nil {
				t.Fatal(err)
			}
			if got.network != test.network || got.host != test.host || got.port != test.port || got.fd != test.fd {
				t.Fatalf("target = %#v", got)
			}
		})
	}
}

func TestParsePortTargetRejectsDescriptorAboveLimit(t *testing.T) {
	_, err := parsePortTarget(fmt.Sprintf("8080=%d", maxInheritedListenerFD+1))
	require.ErrorContains(t, err, fmt.Sprintf("exceeds maximum %d", maxInheritedListenerFD))
}

func TestParsePortTargetReturnsDelimitedTargetParseErrors(t *testing.T) {
	for _, test := range []struct {
		name, spec, wantErr string
	}{
		{name: "malformed IPv6 address", spec: "[::1;next", wantErr: "invalid address"},
		{name: "non-numeric port", spec: "host;next:not-a-port", wantErr: "invalid syntax"},
		{name: "out-of-range port", spec: "host;next:65536", wantErr: "must be between 0 and 65535"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := parsePortTarget(test.spec)
			require.ErrorContains(t, err, test.wantErr)
		})
	}
}

func TestAssignListenerDescriptors(t *testing.T) {
	t.Run("fills automatic descriptors around explicit descriptors", func(t *testing.T) {
		descriptors, err := assignListenerDescriptors([]int{-1, 5, -1})
		require.NoError(t, err)
		require.Equal(t, []int{3, 5, 4}, descriptors)
	})

	t.Run("accepts the sparse padding boundary", func(t *testing.T) {
		descriptors, err := assignListenerDescriptors([]int{maxSparseListenerFDSlots + 3})
		require.NoError(t, err)
		require.Equal(t, []int{maxSparseListenerFDSlots + 3}, descriptors)
	})

	t.Run("rejects excessive sparse padding", func(t *testing.T) {
		_, err := assignListenerDescriptors([]int{maxSparseListenerFDSlots + 4})
		require.ErrorContains(t, err, fmt.Sprintf("maximum is %d", maxSparseListenerFDSlots))
	})

	t.Run("rejects duplicate explicit descriptors", func(t *testing.T) {
		_, err := assignListenerDescriptors([]int{3, 3})
		require.ErrorContains(t, err, "specified more than once")
	})
}

func TestParsePortTargetPortBoundaries(t *testing.T) {
	for _, spec := range []string{"0", "65535"} {
		_, err := parsePortTarget(spec)
		require.NoError(t, err)
	}

	for _, spec := range []string{"-1", "65536"} {
		_, err := parsePortTarget(spec)
		require.EqualError(t, err, "invalid port in \""+spec+"\"")
	}
}

func TestRunRejectsStandardStreamDescriptors(t *testing.T) {
	for _, test := range []struct {
		spec string
		fd   string
	}{
		{spec: "8080=0", fd: "0"},
		{spec: "8080=1", fd: "1"},
		{spec: "8080=2", fd: "2"},
	} {
		s := &Starter{ports: []string{test.spec}, stderr: io.Discard}
		// testing.T.Context is unavailable at the module's Go 1.23 floor.
		ctrl, err := s.Run(context.Background())
		require.EqualError(t, err,
			"invalid file descriptor in \""+test.spec+"\": listener descriptor "+test.fd+
				" conflicts with standard streams")
		require.Nil(t, ctrl)
	}
}
