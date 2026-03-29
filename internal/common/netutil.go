package common

import (
	"io"
	"net"
	"strconv"
	"strings"
)

// NormalizeLocalAddr converts a bare port like "8080" to "127.0.0.1:8080".
// If the input already contains a colon, it is returned as-is.
func NormalizeLocalAddr(addr string) string {
	if strings.Contains(addr, ":") {
		return addr
	}
	if _, err := strconv.Atoi(addr); err == nil {
		return "127.0.0.1:" + addr
	}
	return addr
}

// PipeConns performs bidirectional copy between two connections.
// When either direction finishes, both connections are closed.
func PipeConns(a, b net.Conn) {
	defer a.Close()
	defer b.Close()

	done := make(chan struct{}, 2)

	go func() {
		io.Copy(a, b)
		done <- struct{}{}
	}()

	go func() {
		io.Copy(b, a)
		done <- struct{}{}
	}()

	<-done
}
