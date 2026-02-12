package docker

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/nstogner/operative/pkg/sandbox"
	sandboxv1 "github.com/nstogner/operative/pkg/sandbox/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	// LabelManager is the label used to identify containers managed by this system.
	LabelManager = "manager"
	// LabelManagerValue is the value of the manager label.
	LabelManagerValue = "operativesystem"
	// LabelOperativeID is the label used to identify which operative a container belongs to.
	LabelOperativeID = "operative-id"
	// SandboxImage is the default sandbox container image.
	SandboxImage = "sandbox-python:latest"
	// ServerPort is the gRPC port exposed by the sandbox container.
	ServerPort = "8000"
	// ReconcileInterval is how often the Run loop checks for drift.
	ReconcileInterval = 10 * time.Second
)

// Manager implements sandbox.Manager using Docker containers with gRPC.
type Manager struct {
	client *client.Client
	image  string
}

// Verify interface compliance.
var _ sandbox.Manager = (*Manager)(nil)

// New creates a new Docker sandbox manager.
func New() (*Manager, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("creating docker client: %w", err)
	}
	return &Manager{client: cli, image: SandboxImage}, nil
}

// Run starts a long-running reconciliation loop. It periodically lists
// known operatives and ensures each has a running sandbox container.
// Orphan containers (not matching any known operative) are stopped.
// Blocks until ctx is cancelled.
func (m *Manager) Run(ctx context.Context, operatives sandbox.OperativeLister) error {
	slog.Info("Sandbox manager reconciliation loop starting")

	// Reconcile immediately on start.
	if err := m.reconcile(ctx, operatives); err != nil {
		slog.Error("Initial reconciliation failed", "error", err)
	}

	ticker := time.NewTicker(ReconcileInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("Sandbox manager reconciliation loop stopping")
			return ctx.Err()
		case <-ticker.C:
			if err := m.reconcile(ctx, operatives); err != nil {
				slog.Error("Reconciliation failed", "error", err)
			}
		}
	}
}

// reconcile compares running containers to known operatives and reconciles.
func (m *Manager) reconcile(ctx context.Context, operatives sandbox.OperativeLister) error {
	ids, err := operatives.ListIDs(ctx)
	if err != nil {
		return fmt.Errorf("listing operative IDs: %w", err)
	}

	allContainers, err := m.listAllManagedContainers(ctx)
	if err != nil {
		return fmt.Errorf("listing managed containers: %w", err)
	}

	knownSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		knownSet[id] = true
	}

	runningSet := make(map[string]bool)

	// Stop containers for unknown operatives.
	for _, c := range allContainers {
		opID := c.Labels[LabelOperativeID]
		runningSet[opID] = true
		if !knownSet[opID] {
			slog.Info("Stopping orphaned sandbox", "operativeID", opID)
			m.stopContainer(ctx, opID)
		}
	}

	// Start containers for known operatives that aren't running.
	for _, id := range ids {
		if !runningSet[id] {
			slog.Info("Starting sandbox for operative", "operativeID", id)
			if _, err := m.createAndStart(ctx, id); err != nil {
				slog.Error("Failed to start sandbox", "operativeID", id, "error", err)
			}
		}
	}

	return nil
}

// RunCell executes a code cell in the operative's sandbox via gRPC.
// The sandbox must already be running (started by the Run loop).
// Returns an error if the container is not running.
func (m *Manager) RunCell(ctx context.Context, operativeID, code string, delegate sandbox.Delegate) (*sandbox.Result, error) {
	hostPort, err := m.getRunningPort(ctx, operativeID)
	if err != nil {
		return nil, fmt.Errorf("sandbox not running for operative %s: %w", operativeID, err)
	}

	// Dial gRPC to the sandbox container.
	conn, err := grpc.NewClient(
		fmt.Sprintf("127.0.0.1:%s", hostPort),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("dialing sandbox: %w", err)
	}
	defer conn.Close()

	sbClient := sandboxv1.NewSandboxClient(conn)

	stream, err := sbClient.RunStream(ctx)
	if err != nil {
		return nil, fmt.Errorf("starting stream: %w", err)
	}

	// Send code execution request.
	if err := stream.Send(&sandboxv1.ClientMessage{
		Payload: &sandboxv1.ClientMessage_RunCell{
			RunCell: &sandboxv1.RunCellRequest{
				Code: code,
			},
		},
	}); err != nil {
		return nil, fmt.Errorf("sending run cell request: %w", err)
	}

	// Process the stream: handle output, prompt callbacks, and final result.
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
			slog.Debug("sandbox output", "text", payload.Output.Text, "stderr", payload.Output.IsStderr)

		case *sandboxv1.ServerMessage_RunCellResult:
			return &sandbox.Result{
				Output: payload.RunCellResult.Output,
				Stdout: payload.RunCellResult.Stdout,
				Stderr: payload.RunCellResult.Stderr,
			}, nil

		case *sandboxv1.ServerMessage_PromptModel:
			resp, err := delegate.PromptModel(ctx, payload.PromptModel.Prompt)
			responseVal := resp
			if err != nil {
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
				return nil, fmt.Errorf("sending prompt response: %w", err)
			}

		case *sandboxv1.ServerMessage_PromptSelf:
			if err := delegate.PromptSelf(ctx, payload.PromptSelf.Message); err != nil {
				slog.Error("prompt self failed", "error", err)
			}
		}
	}

	return nil, fmt.Errorf("stream ended without result")
}

// Status returns the status of the operative's sandbox.
func (m *Manager) Status(ctx context.Context, operativeID string) (string, error) {
	containers, err := m.listContainers(ctx, operativeID)
	if err != nil {
		return "unknown", err
	}
	if len(containers) == 0 {
		return "stopped", nil
	}
	return containers[0].State, nil
}

// Close releases the Docker client resources.
func (m *Manager) Close() error {
	return m.client.Close()
}

// --- internal helpers ---

// getRunningPort returns the host port for a running container, or error if not running.
func (m *Manager) getRunningPort(ctx context.Context, operativeID string) (string, error) {
	containerName := m.containerName(operativeID)
	c, err := m.client.ContainerInspect(ctx, containerName)
	if err != nil {
		return "", fmt.Errorf("container not found: %w", err)
	}
	if !c.State.Running {
		return "", fmt.Errorf("container exists but not running (state: %s)", c.State.Status)
	}
	return m.getPort(c)
}

// createAndStart creates a new sandbox container and starts it.
func (m *Manager) createAndStart(ctx context.Context, operativeID string) (string, error) {
	// Ensure image exists locally.
	_, _, err := m.client.ImageInspectWithRaw(ctx, m.image)
	if err != nil {
		return "", fmt.Errorf("sandbox image '%s' not found — run 'make build-sandbox': %w", m.image, err)
	}

	cfg := &container.Config{
		Image: m.image,
		Labels: map[string]string{
			LabelManager:     LabelManagerValue,
			LabelOperativeID: operativeID,
		},
		ExposedPorts: nat.PortSet{
			nat.Port(ServerPort + "/tcp"): {},
		},
	}

	hostCfg := &container.HostConfig{
		PortBindings: nat.PortMap{
			nat.Port(ServerPort + "/tcp"): []nat.PortBinding{
				{
					HostIP:   "127.0.0.1",
					HostPort: "0", // Dynamically assigned port.
				},
			},
		},
	}

	containerName := m.containerName(operativeID)
	resp, err := m.client.ContainerCreate(ctx, cfg, hostCfg, nil, nil, containerName)
	if err != nil {
		return "", fmt.Errorf("creating container: %w", err)
	}

	if err := m.client.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return "", fmt.Errorf("starting container: %w", err)
	}

	c, err := m.client.ContainerInspect(ctx, resp.ID)
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
	slog.Info("Sandbox started", "operativeID", operativeID, "port", port)
	return port, nil
}

// stopContainer stops and removes a container for the given operative.
func (m *Manager) stopContainer(ctx context.Context, operativeID string) {
	containers, err := m.listContainers(ctx, operativeID)
	if err != nil {
		slog.Warn("Failed to list containers for stop", "operativeID", operativeID, "error", err)
		return
	}
	for _, c := range containers {
		timeout := 10
		if err := m.client.ContainerStop(ctx, c.ID, container.StopOptions{Timeout: &timeout}); err != nil {
			slog.Warn("Failed to stop container", "id", c.ID, "error", err)
		}
		if err := m.client.ContainerRemove(ctx, c.ID, types.ContainerRemoveOptions{Force: true}); err != nil {
			slog.Warn("Failed to remove container", "id", c.ID, "error", err)
		}
	}
}

func (m *Manager) containerName(operativeID string) string {
	return "operative-sandbox-" + operativeID
}

func (m *Manager) getPort(c types.ContainerJSON) (string, error) {
	ports := c.NetworkSettings.Ports[nat.Port(ServerPort+"/tcp")]
	if len(ports) > 0 {
		return ports[0].HostPort, nil
	}
	return "", fmt.Errorf("container running but port not mapped")
}

func (m *Manager) waitForHealth(ctx context.Context, port string) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCtx.Done():
			return fmt.Errorf("timeout waiting for sandbox gRPC port")
		case <-ticker.C:
			dialCtx, dialCancel := context.WithTimeout(timeoutCtx, 1*time.Second)
			conn, err := grpc.DialContext(dialCtx, fmt.Sprintf("127.0.0.1:%s", port),
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithBlock(),
			)
			dialCancel()
			if err == nil {
				conn.Close()
				return nil
			}
		}
	}
}

func (m *Manager) listContainers(ctx context.Context, operativeID string) ([]types.Container, error) {
	return m.client.ContainerList(ctx, types.ContainerListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", LabelManager+"="+LabelManagerValue),
			filters.Arg("label", LabelOperativeID+"="+operativeID),
		),
	})
}

func (m *Manager) listAllManagedContainers(ctx context.Context) ([]types.Container, error) {
	return m.client.ContainerList(ctx, types.ContainerListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", LabelManager+"="+LabelManagerValue),
		),
	})
}
