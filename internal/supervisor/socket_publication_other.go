//go:build !linux && !darwin

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
) (net.Listener, socketIdentity, error) {
	return nil, socketIdentity{}, errSafeSocketCleanupUnavailable
}
