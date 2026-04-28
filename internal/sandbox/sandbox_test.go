package sandbox

import (
	"context"
	"errors"
	"os/exec"
	"runtime"
	"testing"
)

func TestNoopReturnsUnavailable(t *testing.T) {
	t.Parallel()
	err := Noop{}.Run(context.Background(), Spec{Image: "img", Command: []string{"sh"}})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Noop.Run = %v, want ErrUnavailable", err)
	}
}

func TestDetectOnNonDarwinReturnsUnavailable(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "darwin" {
		t.Skip("test only meaningful on non-darwin platforms")
	}
	_, err := Detect()
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Detect on %s = %v, want ErrUnavailable", runtime.GOOS, err)
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
	} else {
		if !errors.Is(err, ErrUnavailable) {
			t.Fatalf("Detect without container = %v, want ErrUnavailable", err)
		}
	}
}
