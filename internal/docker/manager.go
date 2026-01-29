// Package docker provides container lifecycle management for configbench.
package docker

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// ContainerOpts configures a Docker container for creation.
type ContainerOpts struct {
	// Image is the container image (e.g., "ghcr.io/org/image:tag").
	Image string

	// Name is an optional container name. If empty, Docker assigns one.
	Name string

	// Volumes maps named volume names to mount paths inside the container.
	// Example: {"zephyr-sdk": "/usr/local/zephyr-sdk"}
	Volumes map[string]string

	// Binds maps host paths to container paths (bind mounts).
	// Example: {"/host/path": "/container/path"}
	Binds map[string]string

	// Env sets environment variables inside the container.
	Env map[string]string

	// WorkDir sets the working directory inside the container.
	WorkDir string

	// Entrypoint overrides the container entrypoint.
	Entrypoint string

	// Cmd is the command to run. If empty, uses the image default.
	Cmd []string
}

// Manager handles Docker container lifecycle operations.
type Manager struct {
	// DockerBin is the path to the docker binary. Defaults to "docker".
	DockerBin string
}

// NewManager creates a Manager using the docker binary from PATH.
func NewManager() *Manager {
	return &Manager{DockerBin: "docker"}
}

// buildCreateArgs constructs the argument list for `docker create`.
func (m *Manager) buildCreateArgs(opts ContainerOpts) []string {
	args := []string{"create"}

	if opts.Name != "" {
		args = append(args, "--name", opts.Name)
	}

	for src, dst := range opts.Volumes {
		args = append(args, "--mount", fmt.Sprintf("type=volume,src=%s,dst=%s", src, dst))
	}

	for hostPath, containerPath := range opts.Binds {
		args = append(args, "--mount", fmt.Sprintf("type=bind,src=%s,dst=%s", hostPath, containerPath))
	}

	for k, v := range opts.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	if opts.WorkDir != "" {
		args = append(args, "-w", opts.WorkDir)
	}

	if opts.Entrypoint != "" {
		args = append(args, "--entrypoint", opts.Entrypoint)
	}

	args = append(args, opts.Image)
	args = append(args, opts.Cmd...)

	return args
}

// CreateContainer creates a new container and returns its ID.
func (m *Manager) CreateContainer(opts ContainerOpts) (string, error) {
	args := m.buildCreateArgs(opts)

	cmd := exec.Command(m.DockerBin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker create: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	return strings.TrimSpace(stdout.String()), nil
}

// StartContainer starts a created container.
func (m *Manager) StartContainer(id string) error {
	cmd := exec.Command(m.DockerBin, "start", id)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker start: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// ExecCommand runs a command inside a running container and returns
// the combined stdout/stderr output and exit code.
func (m *Manager) ExecCommand(containerID string, command []string) (string, int, error) {
	args := append([]string{"exec", containerID}, command...)
	cmd := exec.Command(m.DockerBin, args...)

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return output.String(), -1, fmt.Errorf("docker exec: %w", err)
		}
	}

	return output.String(), exitCode, nil
}

// CopyToContainer copies a file or directory from the host into a container
// using `docker cp`.
func (m *Manager) CopyToContainer(containerID, hostPath, containerPath string) error {
	// docker cp <hostPath> <container>:<containerPath>
	target := containerID + ":" + containerPath
	cmd := exec.Command(m.DockerBin, "cp", hostPath, target)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker cp: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// RemoveContainer removes a container. If force is true, the container
// is killed first if running.
func (m *Manager) RemoveContainer(id string, force bool) error {
	args := []string{"rm"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, id)

	cmd := exec.Command(m.DockerBin, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker rm: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
