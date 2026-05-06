package sandbox

import (
	"reflect"
	"strings"
	"testing"
)

func TestDockerBuildArgv(t *testing.T) {
	t.Parallel()

	d := &dockerBackend{binary: "docker"}

	tests := []struct {
		name string
		spec Spec
		want []string
	}{
		{
			name: "minimal",
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
			name: "test-execution phase: isolated network, no env",
			spec: Spec{
				Image:      "buttress-runner-node:20",
				Command:    []string{"npm", "test"},
				WorkingDir: "/work",
				Mounts: []Mount{
					{HostPath: "/tmp/buttress-run-xyz", GuestPath: "/work"},
				},
				Network: Network{
					Name: "buttress-offline",
					// DisableDNS is silently ignored for Docker — no equivalent flag.
					DisableDNS: true,
				},
				ReadOnlyRootFS:      true,
				DropAllCapabilities: true,
			},
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
			name: "multiple mounts preserves order",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := d.buildArgv(tt.spec)
			if err != nil {
				t.Fatalf("buildArgv: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("argv mismatch\n got: %q\nwant: %q", got, tt.want)
			}
		})
	}
}

func TestDockerBuildArgvRejectsInvalidSpec(t *testing.T) {
	t.Parallel()

	d := &dockerBackend{binary: "docker"}

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
			name: "colon in host path",
			spec: Spec{
				Image:   "img",
				Command: []string{"sh"},
				Mounts:  []Mount{{HostPath: "/has:colon", GuestPath: "/ok"}},
			},
			wantError: "may not contain ':' or ','",
		},
		{
			name: "env key with =",
			spec: Spec{
				Image:   "img",
				Command: []string{"sh"},
				Env:     map[string]string{"BAD=KEY": "v"},
			},
			wantError: "env key",
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
			_, err := d.buildArgv(tt.spec)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantError)
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantError)
			}
		})
	}
}
