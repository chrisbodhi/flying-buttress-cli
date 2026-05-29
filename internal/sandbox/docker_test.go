package sandbox

import (
	"context"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

// newTestDocker returns a dockerBackend wired to a fake binary name so tests
// never invoke the real Docker daemon.
func newTestDocker() *dockerBackend { return &dockerBackend{binary: "docker"} }

// assertArgv calls buildArgv and fails the test if it returns an error or the
// result does not match want.
func assertArgv(t *testing.T, d *dockerBackend, spec Spec, want []string) {
	t.Helper()
	got, err := d.buildArgv(spec)
	if err != nil {
		t.Fatalf("buildArgv returned unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("argv mismatch\n got: %q\nwant: %q", got, want)
	}
}

// assertArgvErr calls buildArgv and fails the test if it succeeds or if the
// error message does not contain wantSub.
func assertArgvErr(t *testing.T, d *dockerBackend, spec Spec, wantSub string) {
	t.Helper()
	_, err := d.buildArgv(spec)
	if err == nil {
		t.Fatalf("buildArgv succeeded; expected error containing %q", wantSub)
	}
	if !strings.Contains(err.Error(), wantSub) {
		t.Errorf("error %q does not contain %q", err.Error(), wantSub)
	}
}

func TestDockerBuildArgv(t *testing.T) {
	t.Parallel()

	d := newTestDocker()

	tests := []struct {
		name string
		spec Spec
		want []string
	}{
		{
			name: "minimal: image and command only",
			spec: Spec{
				Image:   "buttress-runner-node:20",
				Command: []string{"node", "/work/gen.js"},
			},
			want: []string{
				"run", "--rm",
				"buttress-runner-node:20",
				"node", "/work/gen.js",
			},
		},
		{
			name: "generation phase: spec mounted RO, output mounted RW, single env var",
			spec: Spec{
				Image:      "buttress-runner-node:20",
				Command:    []string{"/usr/local/bin/generate"},
				WorkingDir: "/work",
				User:       "1000:1000",
				Mounts: []Mount{
					{HostPath: "/home/b/.cache/buttress/packages/@org/pkg/abc", GuestPath: "/spec", ReadOnly: true},
					{HostPath: "/home/b/code/proj/src/generated", GuestPath: "/out"},
				},
				Env: map[string]string{
					"LLM_API_KEY": "sk-redacted",
				},
				ReadOnlyRootFS:      true,
				DropAllCapabilities: true,
			},
			want: []string{
				"run", "--rm",
				"--user", "1000:1000",
				"-w", "/work",
				"--read-only",
				"--cap-drop", "ALL",
				"-v", "/home/b/.cache/buttress/packages/@org/pkg/abc:/spec:ro",
				"-v", "/home/b/code/proj/src/generated:/out",
				"-e", "LLM_API_KEY=sk-redacted",
				"buttress-runner-node:20",
				"/usr/local/bin/generate",
			},
		},
		{
			name: "test-execution phase: named network, DisableDNS silently ignored",
			spec: Spec{
				Image:      "buttress-runner-node:20",
				Command:    []string{"npm", "test"},
				WorkingDir: "/work",
				Mounts: []Mount{
					{HostPath: "/tmp/buttress-run-xyz", GuestPath: "/work"},
				},
				Network: Network{
					Name:       "buttress-offline",
					DisableDNS: true,
				},
				ReadOnlyRootFS:      true,
				DropAllCapabilities: true,
			},
			// --no-dns must NOT appear; Docker has no such flag.
			want: []string{
				"run", "--rm",
				"-w", "/work",
				"--read-only",
				"--cap-drop", "ALL",
				"--network", "buttress-offline",
				"-v", "/tmp/buttress-run-xyz:/work",
				"buttress-runner-node:20",
				"npm", "test",
			},
		},
		{
			// Regression: DisableDNS alone (no network name) must not emit any
			// network flag. Docker has no --no-dns equivalent.
			name: "DisableDNS without network name emits no network flags",
			spec: Spec{
				Image:   "img",
				Command: []string{"sh"},
				Network: Network{DisableDNS: true},
			},
			want: []string{
				"run", "--rm",
				"img",
				"sh",
			},
		},
		{
			name: "named network without DisableDNS emits only --network",
			spec: Spec{
				Image:   "img",
				Command: []string{"sh"},
				Network: Network{Name: "my-net"},
			},
			want: []string{
				"run", "--rm",
				"--network", "my-net",
				"img",
				"sh",
			},
		},
		{
			name: "env keys sorted deterministically",
			spec: Spec{
				Image:   "img",
				Command: []string{"sh"},
				Env: map[string]string{
					"ZED":   "3",
					"ALPHA": "1",
					"MID":   "2",
				},
			},
			want: []string{
				"run", "--rm",
				"-e", "ALPHA=1",
				"-e", "MID=2",
				"-e", "ZED=3",
				"img",
				"sh",
			},
		},
		{
			name: "empty env map emits no -e flags",
			spec: Spec{
				Image:   "img",
				Command: []string{"sh"},
				Env:     map[string]string{},
			},
			want: []string{
				"run", "--rm",
				"img",
				"sh",
			},
		},
		{
			name: "multiple mounts preserves declaration order",
			spec: Spec{
				Image:   "img",
				Command: []string{"sh"},
				Mounts: []Mount{
					{HostPath: "/src/a", GuestPath: "/a", ReadOnly: true},
					{HostPath: "/src/b", GuestPath: "/b"},
					{HostPath: "/src/c", GuestPath: "/c", ReadOnly: true},
				},
			},
			want: []string{
				"run", "--rm",
				"-v", "/src/a:/a:ro",
				"-v", "/src/b:/b",
				"-v", "/src/c:/c:ro",
				"img",
				"sh",
			},
		},
		{
			name: "multi-word command passed through verbatim",
			spec: Spec{
				Image:   "img",
				Command: []string{"go", "test", "-race", "-count=1", "./..."},
			},
			want: []string{
				"run", "--rm",
				"img",
				"go", "test", "-race", "-count=1", "./...",
			},
		},
		{
			// All hardening flags together with no optional fields: confirms
			// that ReadOnlyRootFS and DropAllCapabilities are independent.
			name: "hardened: read-only rootfs and drop-all-caps without mounts or env",
			spec: Spec{
				Image:               "img",
				Command:             []string{"sh"},
				ReadOnlyRootFS:      true,
				DropAllCapabilities: true,
			},
			want: []string{
				"run", "--rm",
				"--read-only",
				"--cap-drop", "ALL",
				"img",
				"sh",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertArgv(t, d, tt.spec, tt.want)
		})
	}
}

func TestDockerBuildArgvRejectsInvalidSpec(t *testing.T) {
	t.Parallel()

	d := newTestDocker()

	tests := []struct {
		name      string
		spec      Spec
		wantError string
	}{
		{
			name:      "missing image",
			spec:      Spec{Command: []string{"sh"}},
			wantError: "spec.Image is required",
		},
		{
			name:      "missing command",
			spec:      Spec{Image: "img"},
			wantError: "spec.Command must have at least one element",
		},
		{
			name: "relative host path",
			spec: Spec{
				Image:   "img",
				Command: []string{"sh"},
				Mounts:  []Mount{{HostPath: "rel/path", GuestPath: "/abs"}},
			},
			wantError: "HostPath must be absolute",
		},
		{
			name: "relative guest path",
			spec: Spec{
				Image:   "img",
				Command: []string{"sh"},
				Mounts:  []Mount{{HostPath: "/abs", GuestPath: "rel"}},
			},
			wantError: "GuestPath must be absolute",
		},
		{
			// Colon in -v would be parsed as the path separator by Docker.
			name: "colon in host path would corrupt -v syntax",
			spec: Spec{
				Image:   "img",
				Command: []string{"sh"},
				Mounts:  []Mount{{HostPath: "/has:colon", GuestPath: "/ok"}},
			},
			wantError: "may not contain ':' or ','",
		},
		{
			// Comma in guest path is also ambiguous in extended -v syntax.
			name: "comma in guest path would corrupt -v syntax",
			spec: Spec{
				Image:   "img",
				Command: []string{"sh"},
				Mounts:  []Mount{{HostPath: "/ok", GuestPath: "/has,comma"}},
			},
			wantError: "may not contain ':' or ','",
		},
		{
			name: "env key with = would break -e KEY=VALUE encoding",
			spec: Spec{
				Image:   "img",
				Command: []string{"sh"},
				Env:     map[string]string{"BAD=KEY": "v"},
			},
			wantError: "env key",
		},
		{
			name: "empty env key is invalid",
			spec: Spec{
				Image:   "img",
				Command: []string{"sh"},
				Env:     map[string]string{"": "v"},
			},
			wantError: "env key",
		},
		{
			name: "NUL byte in env value would truncate the variable inside the container",
			spec: Spec{
				Image:   "img",
				Command: []string{"sh"},
				Env:     map[string]string{"KEY": "val\x00ue"},
			},
			wantError: "NUL byte",
		},
		{
			name: "relative working directory",
			spec: Spec{
				Image:      "img",
				Command:    []string{"sh"},
				WorkingDir: "work",
			},
			wantError: "WorkingDir must be absolute",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertArgvErr(t, d, tt.spec, tt.wantError)
		})
	}
}

// TestDockerRun_InvalidSpec covers the early-return path in Run when buildArgv
// rejects the spec. No Docker daemon is needed.
func TestDockerRun_InvalidSpec(t *testing.T) {
	t.Parallel()
	d := &dockerBackend{binary: "/nonexistent"}
	err := d.Run(context.Background(), Spec{}) // missing Image and Command
	if err == nil {
		t.Fatal("expected error from invalid spec")
	}
}

// TestDockerRun_ExecPath covers the exec path in Run using a real binary that
// exits immediately. The command will fail (wrong args) but the code path
// through Run is fully exercised.
func TestDockerRun_ExecPath(t *testing.T) {
	t.Parallel()
	truePath, err := exec.LookPath("true")
	if err != nil {
		t.Skip("'true' not found on PATH")
	}
	d := &dockerBackend{binary: truePath}
	// 'true' ignores all arguments and exits 0.
	_ = d.Run(context.Background(), Spec{Image: "img", Command: []string{"sh"}})
}

// TestDockerBuildArgvNeverEmitsNoDNS confirms that --no-dns never appears in
// any Docker argv, regardless of how Network.DisableDNS is set.
func TestDockerBuildArgvNeverEmitsNoDNS(t *testing.T) {
	t.Parallel()

	d := newTestDocker()
	cases := []Spec{
		{Image: "img", Command: []string{"sh"}, Network: Network{DisableDNS: true}},
		{Image: "img", Command: []string{"sh"}, Network: Network{Name: "net", DisableDNS: true}},
		{Image: "img", Command: []string{"sh"}, Network: Network{Name: "net", DisableDNS: false}},
	}
	for _, spec := range cases {
		argv, err := d.buildArgv(spec)
		if err != nil {
			t.Fatalf("buildArgv: %v", err)
		}
		for _, arg := range argv {
			if arg == "--no-dns" {
				t.Errorf("--no-dns appeared in Docker argv %q (Docker has no such flag)", argv)
			}
		}
	}
}
