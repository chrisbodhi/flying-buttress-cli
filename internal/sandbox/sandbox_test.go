package sandbox

import (
	"context"
	"errors"
	"os/exec"
	"runtime"
	"testing"
)

// dockerAvailable reports whether a reachable Docker daemon is present.
func dockerAvailable() bool {
	path, err := exec.LookPath("docker")
	if err != nil {
		return false
	}
	return exec.Command(path, "info", "--format", "{{.ID}}").Run() == nil
}

func TestNoopReturnsUnavailable(t *testing.T) {
	t.Parallel()
	err := Noop{}.Run(context.Background(), Spec{Image: "img", Command: []string{"sh"}})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Noop.Run = %v, want ErrUnavailable", err)
	}
}

// TestDetectReturnsUnavailableWhenNoRuntimePresent only runs when neither the
// Apple container binary nor a reachable Docker daemon is available.
func TestDetectReturnsUnavailableWhenNoRuntimePresent(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "darwin" {
		t.Skip("Apple container may be available on darwin")
	}
	if dockerAvailable() {
		t.Skip("Docker daemon is reachable; Detect will succeed")
	}
	_, err := Detect()
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Detect = %v, want ErrUnavailable", err)
	}
}

// TestDetectOnNonDarwinWithDockerSucceeds verifies that the Docker fallback
// is wired into Detect on non-darwin hosts.
func TestDetectOnNonDarwinWithDockerSucceeds(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "darwin" {
		t.Skip("test targets non-darwin platforms")
	}
	if !dockerAvailable() {
		t.Skip("Docker daemon not reachable")
	}
	sb, err := Detect()
	if err != nil {
		t.Fatalf("Detect with Docker available returned %v, want sandbox", err)
	}
	if sb == nil {
		t.Fatal("Detect returned nil sandbox without error")
	}
}

func TestDetectOnDarwinDependsOnContainerBinary(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "darwin" {
		t.Skip("test only meaningful on darwin")
	}
	_, lookErr := exec.LookPath("container")
	got, err := Detect()
	if lookErr == nil {
		if err != nil {
			t.Fatalf("Detect with container present returned %v, want sandbox", err)
		}
		if got == nil {
			t.Fatal("Detect returned nil sandbox without error")
		}
	} else if !dockerAvailable() {
		// Neither Apple container nor Docker — expect unavailable.
		if !errors.Is(err, ErrUnavailable) {
			t.Fatalf("Detect without any runtime = %v, want ErrUnavailable", err)
		}
	}
	// If Docker is available on darwin the test is ambiguous; we just don't assert.
}

// TestNewDockerBackend_DockerNotFound exercises the LookPath-failure path in
// newDockerBackend. On hosts where docker is not installed the function must
// return ErrUnavailable without panicking.
func TestNewDockerBackend_DockerNotFound(t *testing.T) {
	t.Parallel()
	if dockerAvailable() {
		t.Skip("Docker daemon is reachable; can't test the 'not found' path")
	}
	_, err := newDockerBackend()
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("newDockerBackend() = %v, want ErrUnavailable when docker is absent", err)
	}
}

// TestNewAppleContainer_NonDarwin exercises the non-darwin early-return path.
// On non-darwin hosts this covers the runtime.GOOS check. On darwin it's a
// structural test that confirms the function returns without crashing.
func TestNewAppleContainer_ReturnsUnavailableOnNonDarwin(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "darwin" {
		t.Skip("test only meaningful on non-darwin platforms")
	}
	_, err := newAppleContainer()
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("newAppleContainer() on non-darwin = %v, want ErrUnavailable", err)
	}
}

// TestDetect_ContainerHiddenFallsBackToDocker covers the docker-fallback
// branches in Detect (sandbox.go lines 75-78). We hide the `container` binary
// by clearing PATH so newAppleContainer returns ErrUnavailable, then Detect
// falls through to the docker path (which also fails — no docker daemon either).
func TestDetect_ContainerHiddenFallsBackToDocker(t *testing.T) {
	// Not parallel: mutates PATH.
	if runtime.GOOS != "darwin" {
		t.Skip("test targets the darwin container->docker fallback path")
	}
	t.Setenv("PATH", "")

	_, err := Detect()
	// With no container and no docker on PATH, ErrUnavailable is expected.
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("Detect() with empty PATH = %v, want ErrUnavailable", err)
	}
}

// TestNewAppleContainer_BinaryNotFound covers the LookPath-failure branch when
// the container binary is absent from PATH.
func TestNewAppleContainer_BinaryNotFound(t *testing.T) {
	// Not parallel: mutates PATH.
	if runtime.GOOS != "darwin" {
		t.Skip("test targets the darwin LookPath failure path")
	}
	t.Setenv("PATH", "")

	_, err := newAppleContainer()
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("newAppleContainer() with empty PATH = %v, want ErrUnavailable", err)
	}
}
