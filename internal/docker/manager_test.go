package docker

import (
	"fmt"
	"strings"
	"testing"
)

func TestBuildCreateArgsBase(t *testing.T) {
	m := NewManager()
	args := m.buildCreateArgs(ContainerOpts{
		Image: "alpine:latest",
	})

	if args[0] != "create" {
		t.Errorf("first arg = %q, want 'create'", args[0])
	}
	if args[len(args)-1] != "alpine:latest" {
		t.Errorf("last arg = %q, want 'alpine:latest'", args[len(args)-1])
	}
}

func TestBuildCreateArgsName(t *testing.T) {
	m := NewManager()
	args := m.buildCreateArgs(ContainerOpts{
		Image: "alpine:latest",
		Name:  "test-container",
	})

	assertArgPair(t, args, "--name", "test-container")
}

func TestBuildCreateArgsVolumes(t *testing.T) {
	m := NewManager()
	args := m.buildCreateArgs(ContainerOpts{
		Image: "alpine:latest",
		Volumes: map[string]string{
			"zephyr-sdk": "/usr/local/zephyr-sdk",
		},
	})

	expected := "type=volume,src=zephyr-sdk,dst=/usr/local/zephyr-sdk"
	assertArgPair(t, args, "--mount", expected)
}

func TestBuildCreateArgsMultipleVolumes(t *testing.T) {
	m := NewManager()
	args := m.buildCreateArgs(ContainerOpts{
		Image: "alpine:latest",
		Volumes: map[string]string{
			"vol-a": "/mnt/a",
			"vol-b": "/mnt/b",
		},
	})

	// Both volumes should appear as --mount flags
	mountCount := 0
	for i, a := range args {
		if a == "--mount" && i+1 < len(args) {
			val := args[i+1]
			if strings.HasPrefix(val, "type=volume,") {
				mountCount++
			}
		}
	}
	if mountCount != 2 {
		t.Errorf("expected 2 volume mounts, got %d: %v", mountCount, args)
	}
}

func TestBuildCreateArgsBinds(t *testing.T) {
	m := NewManager()
	args := m.buildCreateArgs(ContainerOpts{
		Image: "alpine:latest",
		Binds: map[string]string{
			"/host/data": "/container/data",
		},
	})

	expected := "type=bind,src=/host/data,dst=/container/data"
	assertArgPair(t, args, "--mount", expected)
}

func TestBuildCreateArgsEnv(t *testing.T) {
	m := NewManager()
	args := m.buildCreateArgs(ContainerOpts{
		Image: "alpine:latest",
		Env: map[string]string{
			"FOO": "bar",
		},
	})

	assertArgPair(t, args, "-e", "FOO=bar")
}

func TestBuildCreateArgsWorkDir(t *testing.T) {
	m := NewManager()
	args := m.buildCreateArgs(ContainerOpts{
		Image:   "alpine:latest",
		WorkDir: "/app",
	})

	assertArgPair(t, args, "-w", "/app")
}

func TestBuildCreateArgsEntrypoint(t *testing.T) {
	m := NewManager()
	args := m.buildCreateArgs(ContainerOpts{
		Image:      "alpine:latest",
		Entrypoint: "/bin/sh",
	})

	assertArgPair(t, args, "--entrypoint", "/bin/sh")
}

func TestBuildCreateArgsCmd(t *testing.T) {
	m := NewManager()
	args := m.buildCreateArgs(ContainerOpts{
		Image: "alpine:latest",
		Cmd:   []string{"echo", "hello"},
	})

	// Image should be followed by command args
	imageIdx := -1
	for i, a := range args {
		if a == "alpine:latest" {
			imageIdx = i
			break
		}
	}
	if imageIdx < 0 {
		t.Fatal("image not found in args")
	}
	if imageIdx+2 >= len(args) {
		t.Fatalf("not enough args after image: %v", args)
	}
	if args[imageIdx+1] != "echo" || args[imageIdx+2] != "hello" {
		t.Errorf("cmd args = %v, want [echo hello]", args[imageIdx+1:])
	}
}

func TestBuildCreateArgsComprehensive(t *testing.T) {
	m := NewManager()
	args := m.buildCreateArgs(ContainerOpts{
		Image:      "ghcr.io/mateosegura/devcontainer-zephyr:latest",
		Name:       "bench-run-1",
		Volumes:    map[string]string{"zephyr-sdk": "/usr/local/zephyr-sdk"},
		Env:        map[string]string{"ZEPHYR_BASE": "/root/zephyrproject/zephyr"},
		WorkDir:    "/root/zephyrproject/zephyr",
		Entrypoint: "/bin/bash",
		Cmd:        []string{"-c", "west build"},
	})

	assertArgPair(t, args, "--name", "bench-run-1")
	assertArgPair(t, args, "--mount", "type=volume,src=zephyr-sdk,dst=/usr/local/zephyr-sdk")
	assertArgPair(t, args, "-e", "ZEPHYR_BASE=/root/zephyrproject/zephyr")
	assertArgPair(t, args, "-w", "/root/zephyrproject/zephyr")
	assertArgPair(t, args, "--entrypoint", "/bin/bash")

	// Image and cmd at the end
	found := false
	for i, a := range args {
		if a == "ghcr.io/mateosegura/devcontainer-zephyr:latest" {
			found = true
			if i+2 >= len(args) || args[i+1] != "-c" || args[i+2] != "west build" {
				t.Errorf("expected cmd after image, got: %v", args[i:])
			}
			break
		}
	}
	if !found {
		t.Errorf("image not found in args: %v", args)
	}
}

func TestNewManager(t *testing.T) {
	m := NewManager()
	if m.DockerBin != "docker" {
		t.Errorf("DockerBin = %q, want 'docker'", m.DockerBin)
	}
}

func TestCopyToContainerArgs(t *testing.T) {
	// We can't run docker cp without a daemon, but we can verify the method
	// exists and has the right signature by calling it with a non-existent
	// binary that will fail predictably.
	m := &Manager{DockerBin: "/nonexistent-docker-binary"}
	err := m.CopyToContainer("abc123", "/host/path", "/container/path")
	if err == nil {
		t.Error("expected error from nonexistent binary")
	}
	// The error should mention docker cp
	if !strings.Contains(err.Error(), "docker cp") {
		t.Errorf("error should mention 'docker cp', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func assertArgPair(t *testing.T, args []string, flag, value string) {
	t.Helper()
	for i, a := range args {
		if a == flag && i+1 < len(args) && args[i+1] == value {
			return
		}
	}
	t.Errorf("args should contain %q %q, got: %v", flag, value, formatArgs(args))
}

func formatArgs(args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = fmt.Sprintf("%q", a)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
