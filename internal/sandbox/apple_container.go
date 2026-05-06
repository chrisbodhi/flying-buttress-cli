package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
)

// appleContainer runs Spec inside Apple's `container` CLI, which boots a
// fresh Linux microVM per invocation via the Virtualization framework.
//
// `container` itself is a host-side binary; its own environment (HOME, PATH,
// vmnet helpers, etc.) is inherited from the parent process. The guest's
// environment is separately and exclusively defined by Spec.Env.
type appleContainer struct {
	binary string
}

func newAppleContainer() (*appleContainer, error) {
	if runtime.GOOS != "darwin" {
		return nil, ErrUnavailable
	}
	path, err := exec.LookPath("container")
	if err != nil {
		return nil, ErrUnavailable
	}
	return &appleContainer{binary: path}, nil
}

func (c *appleContainer) Run(ctx context.Context, spec Spec) error {
	argv, err := c.buildArgv(spec)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, c.binary, argv...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// buildArgv translates a Spec into the argv passed to `container run`. It is
// pure so that tests can pin the exact flag ordering without invoking the
// binary.
func (c *appleContainer) buildArgv(spec Spec) ([]string, error) {
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
	if spec.Network.DisableDNS {
		argv = append(argv, "--no-dns")
	}

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
