package supervisor

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParsePortTarget(t *testing.T) {
	for _, test := range []struct {
		name, spec, network, host string
		port, fd                  int
	}{
		{name: "tcp4", spec: "8080", network: "tcp4", port: 8080, fd: -1},
		{name: "udp4", spec: "u8080", network: "udp4", port: 8080, fd: -1},
		{name: "tcp6", spec: "[::1]:8080", network: "tcp6", host: "::1", port: 8080, fd: -1},
		{name: "udp6", spec: "u[::1]:8080=7", network: "udp6", host: "::1", port: 8080, fd: 7},
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
