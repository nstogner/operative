package docker

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/mariozechner/coding-agent/session/pkg/sandbox"
	sandboxv1 "github.com/mariozechner/coding-agent/session/pkg/sandbox/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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

func (m *DockerManager) RunCell(ctx context.Context, sessionID string, code string, delegate sandbox.Delegate) (*sandbox.Result, error) {
	hostPort, err := m.ensureRunning(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	// Dial gRPC
	conn, err := grpc.Dial(fmt.Sprintf("127.0.0.1:%s", hostPort), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to dial sandbox: %w", err)
	}
	defer conn.Close()

	client := sandboxv1.NewSandboxClient(conn)

	stream, err := client.RunStream(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start stream: %w", err)
	}

	// Send execution request
	if err := stream.Send(&sandboxv1.ClientMessage{
		Payload: &sandboxv1.ClientMessage_RunCell{
			RunCell: &sandboxv1.RunCellRequest{
				Code: code,
			},
		},
	}); err != nil {
		return nil, fmt.Errorf("failed to send run cell request: %w", err)
	}

	var output, stdout, stderr string

	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("stream error: %w", err)
		}

		switch payload := msg.Payload.(type) {
		case *sandboxv1.ServerMessage_Output:
			if payload.Output.IsStderr {
				stderr += payload.Output.Text
			} else {
				stdout += payload.Output.Text
			}
			output += payload.Output.Text

		case *sandboxv1.ServerMessage_RunCellResult:
			// Final result
			// We can append any remaining output if strictly needed, but typically stream sends Output before Result?
			// The result itself has the full output in the proto, but we might have been streaming it.
			// Let's rely on the result for the definitive final string if we want, or build it up.
			// The old implementation returned the full string.
			// The prototype shows Result having output/stdout/stderr.
			return &sandbox.Result{
				Output: payload.RunCellResult.Output,
				Stdout: payload.RunCellResult.Stdout,
				Stderr: payload.RunCellResult.Stderr,
			}, nil

		case *sandboxv1.ServerMessage_PromptModel:
			// Call back to agent
			resp, err := delegate.PromptModel(ctx, payload.PromptModel.Prompt)
			// Send response back
			// Note: If error, we might want to signal that? The proto doesn't have error field for prompt response yet.
			// Assuming success or empty string on error for now, or log it.
			// Ideally we handle error.
			responseVal := resp
			if err != nil {
				// Log error? Send back error message?
				// For now, let's just send the error as the response so the python side sees it, or empty.
				responseVal = fmt.Sprintf("Error: %v", err)
			}

			if err := stream.Send(&sandboxv1.ClientMessage{
				Payload: &sandboxv1.ClientMessage_PromptModelResponse{
					PromptModelResponse: &sandboxv1.PromptModelResponse{
						Id:       payload.PromptModel.Id,
						Response: responseVal,
					},
				},
			}); err != nil {
				return nil, fmt.Errorf("failed to send prompt response: %w", err)
			}

		case *sandboxv1.ServerMessage_PromptSelf:
			// Call back to agent
			if err := delegate.PromptSelf(ctx, payload.PromptSelf.Message); err != nil {
				// Log error
				fmt.Printf("Error prompting self: %v\n", err)
			}
			// No response needed for PromptSelf
		}
	}

	return nil, fmt.Errorf("stream ended without result")
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
	// For gRPC, we could assume if it connects it's healthy, or hit a health endpoint if we implemented grpc-health-probe.
	// For now, let's just try to dial it with a timeout loop?
	// A simple TCP dial check is good.

	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCtx.Done():
			return fmt.Errorf("timeout waiting for sandbox port")
		case <-ticker.C:
			// Try to dial with short timeout
			dialCtx, dialCancel := context.WithTimeout(timeoutCtx, 1*time.Second)
			conn, err := grpc.DialContext(dialCtx, fmt.Sprintf("127.0.0.1:%s", port), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
			dialCancel()

			if err == nil {
				conn.Close()
				return nil
			}
			fmt.Printf("Dial failed: %v\n", err)
			// If error, continue
		}
	}
}
