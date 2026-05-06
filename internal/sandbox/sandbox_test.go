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
