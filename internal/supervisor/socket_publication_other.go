//go:build !linux

package supervisor

import (
	"context"
	"net"
)

type socketPublicationHooks struct{}

func listenFilesystemUnixSocket(
	context.Context,
	string,
	socketPublicationHooks,
) (net.Listener, *socketCleanupState, error) {
	return nil, nil, errSafeSocketCleanupUnavailable
}
