// Package testsupport holds helpers shared by the Go services' integration
// tests. It is test-only: nothing in the production path imports it.
package testsupport

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// RequireContainerRuntime skips the test unless a Docker-compatible daemon is
// reachable. Integration tests must skip cleanly in CI without a runtime, so
// every one of them starts with this call.
func RequireContainerRuntime(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping container integration test in -short mode")
	}
	host := strings.TrimSpace(os.Getenv("DOCKER_HOST"))
	if host == "" {
		if sock := defaultSocket(); sock != "" {
			host = "unix://" + sock
		}
	}
	if host == "" {
		t.Skip("no container runtime: DOCKER_HOST is unset and no default socket exists")
	}
	if err := reachable(host); err != nil {
		t.Skipf("no container runtime: %s is unreachable (%v)", host, err)
	}
}

func defaultSocket() string {
	for _, candidate := range []string{
		"/var/run/docker.sock",
		os.Getenv("HOME") + "/.finch/docker.sock",
	} {
		if candidate == "/.finch/docker.sock" {
			continue
		}
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func reachable(host string) error {
	u, err := url.Parse(host)
	if err != nil {
		return err
	}
	network, address := "tcp", u.Host
	switch u.Scheme {
	case "unix":
		network, address = "unix", u.Path
	case "tcp", "http", "https":
	default:
		network, address = "unix", strings.TrimPrefix(host, "unix://")
	}
	conn, err := net.DialTimeout(network, address, 2*time.Second)
	if err != nil {
		return err
	}
	return conn.Close()
}

// Context returns a context with a generous deadline for pulling and starting
// containers, cancelled when the test ends.
func Context(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	return ctx
}

// FreePort asks the operating system for an unused TCP port on the loopback
// interface. Container tests bind an explicit host port because some
// Docker-compatible daemons (Finch's containerd/CNI stack among them) reject the
// "pick one for me" host port that Testcontainers asks for by default.
func FreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve a free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatalf("release the reserved port: %v", err)
	}
	return port
}

// PortBinding renders the host:container port spec that Testcontainers accepts
// in ContainerRequest.ExposedPorts.
func PortBinding(hostPort, containerPort int) string {
	return fmt.Sprintf("%d:%d/tcp", hostPort, containerPort)
}
