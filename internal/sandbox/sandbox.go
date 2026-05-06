// Package sandbox runs a command inside an isolated environment so that
// generated or otherwise untrusted code cannot read host credentials or write
// outside an explicit output path.
//
// The package is intentionally narrow: it does not know about LLMs, specs, or
// the project layout. A caller assembles a Spec describing exactly which host
// paths to expose, which environment variables to inject, and which network
// (if any) to attach to, and the sandbox runs it.
//
// Two backends are provided: Apple's `container` CLI (macOS-only, preferred
// when available) and Docker (any platform). Detect tries them in that order
// and returns ErrUnavailable only when neither is found.
package sandbox

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// ErrUnavailable is returned by Detect when no sandbox runtime is available.
var ErrUnavailable = errors.New("no sandbox runtime available on this host")

// Mount is a single host-to-guest bind mount.
type Mount struct {
	HostPath  string
	GuestPath string
	ReadOnly  bool
}

// Network describes the network configuration of a sandboxed run.
//
// Apple's `container` does not expose a "no network" mode; the closest
// approximation is to attach to a named network created with no egress route.
// Callers that need an offline phase should create such a network out-of-band
// (`container network create buttress-offline …`) and pass its name here.
type Network struct {
	// Name of the network to attach to. Empty means use the runtime default.
	Name string
	// DisableDNS suppresses DNS configuration inside the guest.
	DisableDNS bool
}

// Spec describes a single sandboxed run.
//
// Env is the *complete* set of environment variables visible inside the
// sandbox; the host process's environment is not inherited. This is the
// primary defense against credential exfiltration: a generator that does not
// receive an API key in Env cannot leak one.
type Spec struct {
	Image               string
	Command             []string
	WorkingDir          string
	Mounts              []Mount
	Env                 map[string]string
	User                string
	Network             Network
	ReadOnlyRootFS      bool
	DropAllCapabilities bool
}

// Sandbox runs commands in an isolated environment.
type Sandbox interface {
	Run(ctx context.Context, spec Spec) error
}

// Detect returns the best sandbox available on this host, or ErrUnavailable.
// Apple's container runtime is preferred; Docker is the fallback.
func Detect() (Sandbox, error) {
	if c, err := newAppleContainer(); err == nil {
		return c, nil
	}
	if d, err := newDockerBackend(); err == nil {
		return d, nil
	}
	return nil, ErrUnavailable
}

// Noop is a Sandbox that runs nothing and returns ErrUnavailable. It exists
// so callers can hold a non-nil Sandbox during wiring before deciding how to
// handle a missing runtime.
type Noop struct{}

func (Noop) Run(context.Context, Spec) error { return ErrUnavailable }

// validate returns a non-nil error if the spec is missing required fields or
// contains values that would be ambiguous when serialized to CLI flags.
func (s Spec) validate() error {
	if s.Image == "" {
		return errors.New("sandbox: spec.Image is required")
	}
	if len(s.Command) == 0 {
		return errors.New("sandbox: spec.Command must have at least one element")
	}
	for i, m := range s.Mounts {
		if !filepath.IsAbs(m.HostPath) {
			return fmt.Errorf("sandbox: mounts[%d].HostPath must be absolute, got %q", i, m.HostPath)
		}
		if !filepath.IsAbs(m.GuestPath) {
			return fmt.Errorf("sandbox: mounts[%d].GuestPath must be absolute, got %q", i, m.GuestPath)
		}
		if strings.ContainsAny(m.HostPath, ":,") || strings.ContainsAny(m.GuestPath, ":,") {
			return fmt.Errorf("sandbox: mounts[%d] paths may not contain ':' or ','", i)
		}
	}
	for k, v := range s.Env {
		if k == "" || strings.ContainsAny(k, "=\x00") {
			return fmt.Errorf("sandbox: env key %q is invalid", k)
		}
		if strings.ContainsAny(v, "\x00") {
			return fmt.Errorf("sandbox: env value for %q contains a NUL byte", k)
		}
	}
	if s.WorkingDir != "" && !filepath.IsAbs(s.WorkingDir) {
		return fmt.Errorf("sandbox: spec.WorkingDir must be absolute, got %q", s.WorkingDir)
	}
	return nil
}
