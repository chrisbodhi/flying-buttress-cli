package sandbox

import (
	"reflect"
	"strings"
	"testing"
)

func TestAppleContainerBuildArgv(t *testing.T) {
	t.Parallel()

	c := &appleContainer{binary: "container"}

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
			name: "generation phase: spec mounted RO, output mounted RW, single env var, default network",
			spec: Spec{
				Image:      "buttress-runner-node:20",
				Command:    []string{"/usr/local/bin/generate"},
				WorkingDir: "/work",
				User:       "1000:1000",
				Mounts: []Mount{
					{HostPath: "/Users/b/.cache/buttress/packages/@org/pkg/abc", GuestPath: "/spec", ReadOnly: true},
					{HostPath: "/Users/b/code/proj/src/generated", GuestPath: "/out"},
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
				"-v", "/Users/b/.cache/buttress/packages/@org/pkg/abc:/spec:ro",
				"-v", "/Users/b/code/proj/src/generated:/out",
				"-e", "LLM_API_KEY=sk-redacted",
				"buttress-runner-node:20",
				"/usr/local/bin/generate",
			},
		},
		{
			name: "test-execution phase: isolated network, no DNS, no env",
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
			want: []string{
				"run", "--rm",
				"-w", "/work",
				"--read-only",
				"--cap-drop", "ALL",
				"--network", "buttress-offline",
				"--no-dns",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := c.buildArgv(tt.spec)
			if err != nil {
				t.Fatalf("buildArgv: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("argv mismatch\n got: %q\nwant: %q", got, tt.want)
			}
		})
	}
}

func TestAppleContainerBuildArgvRejectsInvalidSpec(t *testing.T) {
	t.Parallel()

	c := &appleContainer{binary: "container"}

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
			name: "colon in host path would confuse -v parsing",
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
			_, err := c.buildArgv(tt.spec)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantError)
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantError)
			}
		})
	}
}
