package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/mariozechner/coding-agent/session/pkg/sandbox"
)

const (
	ImageName  = "sandbox-python:latest"
	ServerPort = "8000"
)

// DockerManager implements sandbox.Manager using Docker containers.
type DockerManager struct {
	cli *client.Client
}

// Ensure DockerManager implements sandbox.Manager
var _ sandbox.Manager = (*DockerManager)(nil)

// New creates a new DockerManager.
func New() (*DockerManager, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	return &DockerManager{
		cli: cli,
	}, nil
}

func (m *DockerManager) Close() error {
	return m.cli.Close()
}

func (m *DockerManager) containerName(sessionID string) string {
	return fmt.Sprintf("session-%s", sessionID)
}

func (m *DockerManager) RunCell(ctx context.Context, sessionID string, code string) (*sandbox.Result, error) {
	hostPort, err := m.ensureRunning(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("http://127.0.0.1:%s/tools:run_ipython_cell", hostPort)

	reqBody := map[string]interface{}{
		"code":         code,
		"split_output": false,
	}
	jsonBody, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("sandbox error %d: %s", resp.StatusCode, string(body))
	}

	var res struct {
		Output string `json:"output"`
		Stdout string `json:"stdout"`
		Stderr string `json:"stderr"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	return &sandbox.Result{
		Output: res.Output,
		Stdout: res.Stdout,
		Stderr: res.Stderr,
	}, nil
}

func (m *DockerManager) Stop(ctx context.Context, sessionID string) error {
	return m.cli.ContainerRemove(ctx, m.containerName(sessionID), types.ContainerRemoveOptions{
		Force: true,
	})
}

// ensureRunning checks if the container is running, starts it if not, and returns the host port.
func (m *DockerManager) ensureRunning(ctx context.Context, sessionID string) (string, error) {
	name := m.containerName(sessionID)

	// Check if container exists
	c, err := m.cli.ContainerInspect(ctx, name)
	if err != nil {
		if client.IsErrNotFound(err) {
			// Create it
			return m.createAndStart(ctx, sessionID)
		}
		return "", fmt.Errorf("failed to inspect container: %w", err)
	}

	if c.State.Running {
		// Get port
		port, err := m.getPort(c)
		if err != nil {
			return "", err
		}
		// We assumes it's healthy if running, but maybe we should check health?
		// For performance, we can skip full health check if it's already running,
		// but if it just started it might not be ready.
		// However, for lazy launch, we probably want to be sure.
		if err := m.waitForHealth(ctx, port); err != nil {
			return "", err
		}
		return port, nil
	}

	// Start it if it exists but is stopped
	if err := m.cli.ContainerStart(ctx, name, types.ContainerStartOptions{}); err != nil {
		return "", fmt.Errorf("failed to start container: %w", err)
	}

	// Inspect again to get port
	c, err = m.cli.ContainerInspect(ctx, name)
	if err != nil {
		return "", err
	}
	port, err := m.getPort(c)
	if err != nil {
		return "", err
	}

	if err := m.waitForHealth(ctx, port); err != nil {
		return "", err
	}
	return port, nil
}

func (m *DockerManager) createAndStart(ctx context.Context, sessionID string) (string, error) {
	// Ensure image exists (locally)
	_, _, err := m.cli.ImageInspectWithRaw(ctx, ImageName)
	if err != nil {
		// If not found, try to build or error?
		// User mentioned "use a Makefile to build it". So we assume it's built.
		return "", fmt.Errorf("sandbox image '%s' not found. Please run 'make build-sandbox': %w", ImageName, err)
	}

	cfg := &container.Config{
		Image: ImageName,
		// Cmd is inherited from Dockerfile (python server.py)
		ExposedPorts: nat.PortSet{
			nat.Port(ServerPort + "/tcp"): {},
		},
	}

	hostCfg := &container.HostConfig{
		PortBindings: nat.PortMap{
			nat.Port(ServerPort + "/tcp"): []nat.PortBinding{
				{
					HostIP:   "127.0.0.1",
					HostPort: "0",
				},
			},
		},
	}

	name := m.containerName(sessionID)
	resp, err := m.cli.ContainerCreate(ctx, cfg, hostCfg, nil, nil, name)
	if err != nil {
		return "", fmt.Errorf("failed to create container: %w", err)
	}

	if err := m.cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return "", fmt.Errorf("failed to start container: %w", err)
	}

	// Get port
	c, err := m.cli.ContainerInspect(ctx, resp.ID)
	if err != nil {
		return "", err
	}
	port, err := m.getPort(c)
	if err != nil {
		return "", err
	}

	if err := m.waitForHealth(ctx, port); err != nil {
		return "", err
	}
	return port, nil
}

func (m *DockerManager) getPort(c types.ContainerJSON) (string, error) {
	ports := c.NetworkSettings.Ports[nat.Port(ServerPort+"/tcp")]
	if len(ports) > 0 {
		return ports[0].HostPort, nil
	}
	return "", fmt.Errorf("container running but port not mapped")
}

func (m *DockerManager) waitForHealth(ctx context.Context, port string) error {
	url := fmt.Sprintf("http://127.0.0.1:%s/healthz", port)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	// Initial startup can be slow due to pip install
	timeoutCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	for {
		select {
		case <-timeoutCtx.Done():
			return fmt.Errorf("timeout waiting for sandbox health")
		case <-ticker.C:
			resp, err := http.Get(url)
			if err == nil && resp.StatusCode == 200 {
				resp.Body.Close()
				return nil
			}
		}
	}
}
