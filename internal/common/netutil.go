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
		_, err := io.Copy(a, b)
		if err != nil {
			Debug("pipe %s->%s: %v", b.RemoteAddr(), a.RemoteAddr(), err)
		}
		done <- struct{}{}
	}()

	go func() {
		_, err := io.Copy(b, a)
		if err != nil {
			Debug("pipe %s->%s: %v", a.RemoteAddr(), b.RemoteAddr(), err)
		}
		done <- struct{}{}
	}()

	<-done
}

// PipeRW performs bidirectional copy between two endpoints.
// Each endpoint is described by its reader and writer.
// closeFuncs are called when the pipe finishes to clean up resources.
func PipeRW(r1 io.Reader, w1 io.Writer, r2 io.Reader, w2 io.Writer, closeFuncs ...func()) {
	for _, f := range closeFuncs {
		defer f()
	}

	done := make(chan struct{}, 2)

	go func() {
		_, err := io.Copy(w1, r2)
		if err != nil {
			Debug("pipe r2->w1: %v", err)
		}
		done <- struct{}{}
	}()

	go func() {
		_, err := io.Copy(w2, r1)
		if err != nil {
			Debug("pipe r1->w2: %v", err)
		}
		done <- struct{}{}
	}()

	<-done
}