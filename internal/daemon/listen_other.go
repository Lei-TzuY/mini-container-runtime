//go:build !linux

package daemon

import "net"

func listen(network, address string) (net.Listener, error) {
	return net.Listen(network, address)
}
