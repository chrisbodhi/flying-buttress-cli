package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
)

// dockerBackend runs Spec inside Docker. It is available on any platform where
// the `docker` CLI is installed and the daemon is reachable.
//
// Network.DisableDNS is a no-op for Docker: the CLI has no equivalent of
// Apple `container`'s --no-dns flag. Callers that need DNS-free isolation
// should pass a network name with no DNS configuration at the Docker level.
type dockerBackend struct {
	binary string
}

func newDockerBackend() (*dockerBackend, error) {
	path, err := exec.LookPath("docker")
	if err != nil {
		return nil, ErrUnavailable
	}
	// Probe that the daemon is actually reachable so we don't hand back a
	// backend that will fail on every Run call.
	if err := exec.Command(path, "info", "--format", "{{.ID}}").Run(); err != nil {
		return nil, ErrUnavailable
	}
	return &dockerBackend{binary: path}, nil
}

func (d *dockerBackend) Run(ctx context.Context, spec Spec) error {
	argv, err := d.buildArgv(spec)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, d.binary, argv...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// buildArgv translates a Spec into the argv passed to `docker run`. It is
// pure so that tests can pin the exact flag ordering without invoking Docker.
func (d *dockerBackend) buildArgv(spec Spec) ([]string, error) {
	if err := spec.validate(); err != nil {
		return nil, err
	}

	argv := []string{"run", "--rm"}

	if spec.User != "" {
		argv = append(argv, "--user", spec.User)
	}
	if spec.WorkingDir != "" {
		argv = append(argv, "-w", spec.WorkingDir)
	}
	if spec.ReadOnlyRootFS {
		argv = append(argv, "--read-only")
	}
	if spec.DropAllCapabilities {
		argv = append(argv, "--cap-drop", "ALL")
	}

	if spec.Network.Name != "" {
		argv = append(argv, "--network", spec.Network.Name)
	}
	// Network.DisableDNS has no Docker equivalent; skipped intentionally.

	for _, m := range spec.Mounts {
		v := m.HostPath + ":" + m.GuestPath
		if m.ReadOnly {
			v += ":ro"
		}
		argv = append(argv, "-v", v)
	}

	// Sort env keys so argv is deterministic for callers (tests, audit logs).
	keys := make([]string, 0, len(spec.Env))
	for k := range spec.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		argv = append(argv, "-e", fmt.Sprintf("%s=%s", k, spec.Env[k]))
	}

	argv = append(argv, spec.Image)
	argv = append(argv, spec.Command...)
	return argv, nil
}
