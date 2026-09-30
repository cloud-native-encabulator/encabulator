package testsupport

import (
	"context"
	"net/netip"
	"strconv"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// ContainerSpec describes a backing store to run for an integration test.
type ContainerSpec struct {
	// Image is the image reference, for example "postgres:18".
	Image string
	// Cmd overrides the image command.
	Cmd []string
	// Env is the container environment.
	Env map[string]string
	// Port is the container port to publish.
	Port int
	// Wait is the readiness strategy. Required; set its startup timeout
	// generously so that a cold image pull does not fail the test.
	Wait wait.Strategy
}

// StartContainer runs spec and returns the loopback host port its Port is
// published on. The container is terminated when the test ends.
//
// The host port is chosen up front and bound explicitly rather than left to the
// daemon, because some Docker-compatible daemons (Finch's containerd and CNI
// stack among them) reject the "any free port" binding that Testcontainers asks
// for by default.
func StartContainer(t *testing.T, spec ContainerSpec) int {
	t.Helper()
	RequireContainerRuntime(t)
	ctx := Context(t)

	hostPort := FreePort(t)
	containerPort := network.MustParsePort(strconv.Itoa(spec.Port) + "/tcp")

	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		Started: true,
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        spec.Image,
			Cmd:          spec.Cmd,
			Env:          spec.Env,
			ExposedPorts: []string{strconv.Itoa(spec.Port) + "/tcp"},
			WaitingFor:   spec.Wait,
			HostConfigModifier: func(hc *container.HostConfig) {
				hc.PortBindings = network.PortMap{
					containerPort: []network.PortBinding{{
						HostIP:   netip.AddrFrom4([4]byte{127, 0, 0, 1}),
						HostPort: strconv.Itoa(hostPort),
					}},
				}
			},
		},
	})
	if err != nil {
		t.Fatalf("start %s: %v", spec.Image, err)
	}
	t.Cleanup(func() {
		if err := c.Terminate(context.Background()); err != nil {
			t.Logf("terminate %s: %v", spec.Image, err)
		}
	})
	return hostPort
}
