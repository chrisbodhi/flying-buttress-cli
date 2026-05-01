package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"buttress/internal/config"
	"buttress/internal/generate"
	"buttress/internal/ref"
	"buttress/internal/store"
	"buttress/internal/versions"
)

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

// fakeSpecClient is an in-memory specClient. Tests configure ListVersions/Fetch
// behavior by setting the fields on the instance.
type fakeSpecClient struct {
	listVersions func(ctx context.Context, url string) ([]versions.Version, error)
	fetchSpec    func(ctx context.Context, pkg ref.PackageRef, hash, dest string) (*generate.SpecArchive, error)
}

func (f *fakeSpecClient) ListVersionsFromURL(ctx context.Context, url string) ([]versions.Version, error) {
	return f.listVersions(ctx, url)
}

func (f *fakeSpecClient) FetchSpec(ctx context.Context, pkg ref.PackageRef, hash, dest string) (*generate.SpecArchive, error) {
	return f.fetchSpec(ctx, pkg, hash, dest)
}

// recordingGenerator captures Generate calls and returns a configurable error.
type recordingGenerator struct {
	called   bool
	gotReq   generate.Request
	returnEr error
}

func (g *recordingGenerator) Generate(_ context.Context, req generate.Request) error {
	g.called = true
	g.gotReq = req
	return g.returnEr
}

// addDepsFixture wires up a fully-stubbed deps struct and returns it along with
// the buffers and key state needed for assertions.
type addDepsFixture struct {
	deps     *addDeps
	stdout   *bytes.Buffer
	stderr   *bytes.Buffer
	tempHome string
	cacheDir string
	projDir  string
	gen      *recordingGenerator
	client   *fakeSpecClient
}

// newFixture builds a deps suitable for runAdd that:
//   - returns the given config (or an empty one if nil)
//   - uses an in-memory specClient that successfully fetches a spec into a tmp dir
//   - returns a single-version picker that auto-selects that version
//   - writes outputs to in-memory buffers
//   - persists store state to t.TempDir()
func newFixture(t *testing.T, cfg *config.Config) *addDepsFixture {
	t.Helper()

	if cfg == nil {
		cfg = &config.Config{}
	}

	root := t.TempDir()
	cacheDir := filepath.Join(root, "cache")
	projDir := filepath.Join(root, "proj")
	for _, d := range []string{cacheDir, projDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cfg.Registry.CacheDir = cacheDir

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	gen := &recordingGenerator{}
	client := &fakeSpecClient{
		listVersions: func(_ context.Context, _ string) ([]versions.Version, error) {
			return []versions.Version{
				{Hash: "sha256:newest", Description: "newest"},
				{Hash: "sha256:older", Description: "older"},
			}, nil
		},
		fetchSpec: func(_ context.Context, _ ref.PackageRef, hash, _ string) (*generate.SpecArchive, error) {
			// Ignore the production-determined dest (which lives under
			// /tmp/buttress-* and would collide between parallel tests).
			// Use a per-call temp dir so each test gets fresh state.
			actualDest := filepath.Join(t.TempDir(), "spec")
			if err := os.MkdirAll(actualDest, 0o755); err != nil {
				return nil, err
			}
			if err := os.WriteFile(filepath.Join(actualDest, "spec.md"), []byte("# Spec\n"), 0o644); err != nil {
				return nil, err
			}
			return &generate.SpecArchive{Dir: actualDest, ContentHash: hash}, nil
		},
	}

	deps := &addDeps{
		loadConfig: func() (*config.Config, error) { return cfg, nil },
		newClient:  func() specClient { return client },
		runPicker: func(_ string, vs []versions.Version) (*versions.Version, error) {
			return &vs[0], nil
		},
		versionsURLFor: func(p ref.PackageRef) string {
			return "https://example.invalid/" + p.Org + "/" + p.Pkg + "/VERSIONS.txt"
		},
		newStore: func(_, _ string) (*store.Store, error) {
			// Always anchor the store at the fixture's project dir.
			return store.New(cacheDir, projDir)
		},
		newGenerator: func(_ *config.Config) generate.Generator { return gen },
		getwd:        func() (string, error) { return projDir, nil },
		stdout:       stdout,
		stderr:       stderr,
		now:          func() time.Time { return time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC) },
	}

	return &addDepsFixture{
		deps:     deps,
		stdout:   stdout,
		stderr:   stderr,
		tempHome: root,
		cacheDir: cacheDir,
		projDir:  projDir,
		gen:      gen,
		client:   client,
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestDefaultAddDeps_AllFieldsSet asserts the production deps factory wires up
// every field. This guards against future fields silently defaulting to nil
// and crashing runAdd at runtime.
func TestDefaultAddDeps_AllFieldsSet(t *testing.T) {
	t.Parallel()
	d := defaultAddDeps()
	if d.loadConfig == nil {
		t.Error("loadConfig is nil")
	}
	if d.newClient == nil {
		t.Error("newClient is nil")
	}
	if d.runPicker == nil {
		t.Error("runPicker is nil")
	}
	if d.versionsURLFor == nil {
		t.Error("versionsURLFor is nil")
	}
	if d.newStore == nil {
		t.Error("newStore is nil")
	}
	if d.newGenerator == nil {
		t.Error("newGenerator is nil")
	}
	if d.getwd == nil {
		t.Error("getwd is nil")
	}
	if d.stdout == nil || d.stderr == nil {
		t.Error("stdout/stderr is nil")
	}
	if d.now == nil {
		t.Error("now is nil")
	}
	// Confirm the constructors return non-nil values (smoke test).
	if d.newClient() == nil {
		t.Error("newClient() returned nil")
	}
	if d.versionsURLFor(ref.PackageRef{Org: "o", Pkg: "p"}) == "" {
		t.Error("versionsURLFor returned empty")
	}
	if d.newGenerator(&config.Config{}) == nil {
		t.Error("newGenerator() returned nil")
	}
	if d.now().IsZero() {
		t.Error("now() returned zero time")
	}
}

func TestSanitize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in, want string
	}{
		{"sha256:abc123", "sha256-abc123"},
		{"refs/heads/main", "refs-heads-main"},
		{"plain", "plain"},
		{"", ""},
		{"sha256:abc/def", "sha256-abc-def"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			if got := sanitize(tt.in); got != tt.want {
				t.Errorf("sanitize(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNewAddCmd_Metadata(t *testing.T) {
	t.Parallel()
	c := newAddCmd()
	if c.Use == "" || !strings.HasPrefix(c.Use, "add") {
		t.Errorf("Use = %q", c.Use)
	}
	if c.Short == "" {
		t.Error("Short is empty")
	}
	for _, name := range []string{"versions-url", "generate"} {
		if c.Flag(name) == nil {
			t.Errorf("missing flag --%s", name)
		}
	}
}

func TestNewAddCmd_RequiresExactlyOneArg(t *testing.T) {
	t.Parallel()
	c := newAddCmd()
	// Cobra's Args validator runs against the command directly.
	if err := c.Args(c, []string{}); err == nil {
		t.Error("expected error with 0 args")
	}
	if err := c.Args(c, []string{"@a/b", "@c/d"}); err == nil {
		t.Error("expected error with 2 args")
	}
	if err := c.Args(c, []string{"@a/b"}); err != nil {
		t.Errorf("unexpected error with 1 arg: %v", err)
	}
}

func TestRunAdd_HappyPath_WithExplicitHash(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, nil)
	err := runAdd(context.Background(), fx.deps, "@acme/widget@sha256:abc", "", false)
	if err != nil {
		t.Fatalf("runAdd() error: %v", err)
	}

	// Lock file written.
	lockPath := filepath.Join(fx.projDir, "buttress.lock")
	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("buttress.lock not written: %v", err)
	}
	if !strings.Contains(string(data), "sha256:abc") {
		t.Errorf("lock file missing hash, got: %s", data)
	}

	// Spec is hard-linked into project dir.
	if _, err := os.Stat(filepath.Join(fx.projDir, "buttress", "@acme", "widget", "spec.md")); err != nil {
		t.Errorf("spec.md missing from project: %v", err)
	}

	// Stdout reports success.
	if !strings.Contains(fx.stdout.String(), "added") {
		t.Errorf("stdout missing 'added': %q", fx.stdout.String())
	}
}

func TestRunAdd_HappyPath_WithPicker(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, nil)
	var pickerCalled bool
	fx.deps.runPicker = func(pkg string, vs []versions.Version) (*versions.Version, error) {
		pickerCalled = true
		if pkg != "@acme/widget" {
			t.Errorf("picker pkg = %q, want @acme/widget", pkg)
		}
		if len(vs) == 0 {
			t.Error("picker called with empty versions")
		}
		return &vs[0], nil
	}

	if err := runAdd(context.Background(), fx.deps, "@acme/widget", "", false); err != nil {
		t.Fatalf("runAdd() error: %v", err)
	}
	if !pickerCalled {
		t.Error("picker was not invoked when no hash supplied")
	}
}

func TestRunAdd_PickerCancelled(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, nil)
	fx.deps.runPicker = func(string, []versions.Version) (*versions.Version, error) {
		return nil, nil
	}

	err := runAdd(context.Background(), fx.deps, "@acme/widget", "", false)
	if err == nil || !strings.Contains(err.Error(), "no version selected") {
		t.Errorf("expected 'no version selected' error, got: %v", err)
	}
}

func TestRunAdd_PickerErrors(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, nil)
	want := errors.New("picker failed")
	fx.deps.runPicker = func(string, []versions.Version) (*versions.Version, error) {
		return nil, want
	}

	err := runAdd(context.Background(), fx.deps, "@acme/widget", "", false)
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want errors.Is %v", err, want)
	}
}

func TestRunAdd_VersionsURLOverride(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, nil)
	var seenURL string
	fx.client.listVersions = func(_ context.Context, url string) ([]versions.Version, error) {
		seenURL = url
		return []versions.Version{{Hash: "sha256:x", Description: "only"}}, nil
	}

	if err := runAdd(context.Background(), fx.deps, "@acme/widget", "https://override.example/V.txt", false); err != nil {
		t.Fatalf("runAdd() error: %v", err)
	}
	if seenURL != "https://override.example/V.txt" {
		t.Errorf("ListVersionsFromURL got %q, want override URL", seenURL)
	}
}

func TestRunAdd_RejectsBadRef(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, nil)
	err := runAdd(context.Background(), fx.deps, "not-a-ref", "", false)
	if err == nil {
		t.Fatal("expected error for invalid ref")
	}
}

func TestRunAdd_ConfigLoadError(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, nil)
	want := errors.New("disk error")
	fx.deps.loadConfig = func() (*config.Config, error) { return nil, want }

	err := runAdd(context.Background(), fx.deps, "@a/b@sha256:x", "", false)
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want errors.Is %v", err, want)
	}
}

func TestRunAdd_GenerateRequiresLLM(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, &config.Config{}) // no LLM
	err := runAdd(context.Background(), fx.deps, "@a/b@sha256:x", "", true)
	if err == nil || !strings.Contains(err.Error(), "--generate requires") {
		t.Errorf("err = %v, want substring '--generate requires'", err)
	}
	if fx.gen.called {
		t.Error("generator should not be called when LLM is missing")
	}
}

func TestRunAdd_GenerateInvokesGenerator(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		LLM:     config.LLMConfig{Provider: "ollama", Model: "llama3"},
		Project: config.ProjectConfig{Language: "go"},
	}
	fx := newFixture(t, cfg)
	fx.gen.returnEr = nil

	if err := runAdd(context.Background(), fx.deps, "@a/b@sha256:x", "", true); err != nil {
		t.Fatalf("runAdd() error: %v", err)
	}
	if !fx.gen.called {
		t.Fatal("generator was not called")
	}
	if fx.gen.gotReq.Language != "go" {
		t.Errorf("generator language = %q, want %q", fx.gen.gotReq.Language, "go")
	}
	if fx.gen.gotReq.Spec == nil {
		t.Fatal("generator received nil spec")
	}
	if fx.gen.gotReq.PackageName != "@a/b" {
		t.Errorf("generator PackageName = %q, want @a/b", fx.gen.gotReq.PackageName)
	}
	if fx.gen.gotReq.OutputPath == "" {
		t.Error("generator OutputPath is empty")
	}
	if fx.gen.gotReq.ProjectDir != fx.projDir {
		t.Errorf("generator ProjectDir = %q, want %q", fx.gen.gotReq.ProjectDir, fx.projDir)
	}
}

func TestRunAdd_GenerateNotConfiguredBubblesUp(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		LLM:     config.LLMConfig{Provider: "ollama", Model: "llama3"},
		Project: config.ProjectConfig{Language: "go"},
	}
	fx := newFixture(t, cfg)
	fx.gen.returnEr = generate.ErrNotConfigured

	err := runAdd(context.Background(), fx.deps, "@a/b@sha256:x", "", true)
	if !errors.Is(err, generate.ErrNotConfigured) {
		t.Errorf("err = %v, want errors.Is ErrNotConfigured", err)
	}
}

func TestRunAdd_GenerateOtherError(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		LLM:     config.LLMConfig{Provider: "ollama", Model: "llama3"},
		Project: config.ProjectConfig{Language: "go"},
	}
	fx := newFixture(t, cfg)
	fx.gen.returnEr = errors.New("network down")

	err := runAdd(context.Background(), fx.deps, "@a/b@sha256:x", "", true)
	if err == nil || !strings.Contains(err.Error(), "generation failed") {
		t.Errorf("err = %v, want substring 'generation failed'", err)
	}
}

func TestRunAdd_GenerateMissingLanguage(t *testing.T) {
	t.Parallel()

	// LLM is configured but no language is set (and no .buttress.local.toml).
	// resolveGenOutput should refuse with a helpful error.
	cfg := &config.Config{
		LLM: config.LLMConfig{Provider: "ollama", Model: "llama3"},
	}
	fx := newFixture(t, cfg)

	err := runAdd(context.Background(), fx.deps, "@a/b@sha256:x", "", true)
	if err == nil || !strings.Contains(err.Error(), "no language configured") {
		t.Errorf("err = %v, want substring 'no language configured'", err)
	}
	if fx.gen.called {
		t.Error("generator should not be called when language resolution fails")
	}
}

func TestRunAdd_GenerateGetwdError(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		LLM:     config.LLMConfig{Provider: "ollama", Model: "llama3"},
		Project: config.ProjectConfig{Language: "go"},
	}
	fx := newFixture(t, cfg)
	want := errors.New("disk gone")
	fx.deps.getwd = func() (string, error) { return "", want }

	err := runAdd(context.Background(), fx.deps, "@a/b@sha256:x", "", true)
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want errors.Is %v", err, want)
	}
}

func TestRunAdd_AlreadyInstalled(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, nil)

	// Pre-populate lock with the same hash.
	st, err := store.New(fx.cacheDir, fx.projDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.WriteLock(map[string]store.LockEntry{
		"@acme/widget": {Org: "acme", Pkg: "widget", Hash: "sha256:abc"},
	}); err != nil {
		t.Fatal(err)
	}

	var fetched bool
	fx.client.fetchSpec = func(context.Context, ref.PackageRef, string, string) (*generate.SpecArchive, error) {
		fetched = true
		return nil, errors.New("should not fetch")
	}

	if err := runAdd(context.Background(), fx.deps, "@acme/widget@sha256:abc", "", false); err != nil {
		t.Fatalf("runAdd() error: %v", err)
	}
	if fetched {
		t.Error("FetchSpec called when already installed")
	}
	if !strings.Contains(fx.stdout.String(), "already installed") {
		t.Errorf("stdout = %q, want 'already installed'", fx.stdout.String())
	}
}

func TestRunAdd_ReplacesOldVersion(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, nil)

	st, err := store.New(fx.cacheDir, fx.projDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.WriteLock(map[string]store.LockEntry{
		"@acme/widget": {Org: "acme", Pkg: "widget", Hash: "sha256:OLD"},
	}); err != nil {
		t.Fatal(err)
	}

	if err := runAdd(context.Background(), fx.deps, "@acme/widget@sha256:NEW", "", false); err != nil {
		t.Fatalf("runAdd() error: %v", err)
	}
	if !strings.Contains(fx.stdout.String(), "replacing") {
		t.Errorf("stdout = %q, want 'replacing'", fx.stdout.String())
	}

	// Lock now reflects new hash.
	lock, err := st.ReadLock()
	if err != nil {
		t.Fatal(err)
	}
	if lock["@acme/widget"].Hash != "sha256:NEW" {
		t.Errorf("lock entry hash = %q, want sha256:NEW", lock["@acme/widget"].Hash)
	}
}

func TestRunAdd_ListVersionsError(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, nil)
	want := errors.New("network kaboom")
	fx.client.listVersions = func(context.Context, string) ([]versions.Version, error) {
		return nil, want
	}

	err := runAdd(context.Background(), fx.deps, "@acme/widget", "", false)
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want errors.Is %v", err, want)
	}
	if !strings.Contains(err.Error(), "listing versions") {
		t.Errorf("err = %q, want substring 'listing versions'", err)
	}
}

func TestRunAdd_FetchError(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, nil)
	want := errors.New("404 missing tag")
	fx.client.fetchSpec = func(context.Context, ref.PackageRef, string, string) (*generate.SpecArchive, error) {
		return nil, want
	}

	err := runAdd(context.Background(), fx.deps, "@acme/widget@sha256:abc", "", false)
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want errors.Is %v", err, want)
	}
}

func TestRunAdd_StoreNewError(t *testing.T) {
	t.Parallel()

	fx := newFixture(t, nil)
	want := errors.New("disk failure")
	fx.deps.newStore = func(string, string) (*store.Store, error) { return nil, want }

	err := runAdd(context.Background(), fx.deps, "@a/b@sha256:x", "", false)
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want errors.Is %v", err, want)
	}
}

// ---------------------------------------------------------------------------
// Execute (cmd/root.go) smoke test
// ---------------------------------------------------------------------------

func TestRootCmd_HasAddSubcommand(t *testing.T) {
	t.Parallel()
	root := newRootCmd()
	if root.Use == "" {
		t.Error("root.Use is empty")
	}
	var foundAdd bool
	for _, c := range root.Commands() {
		if c.Name() == "add" {
			foundAdd = true
			break
		}
	}
	if !foundAdd {
		t.Error("root command tree is missing 'add' subcommand")
	}
}

func TestRootCmd_HelpExecutes(t *testing.T) {
	t.Parallel()
	root := newRootCmd()
	root.SetArgs([]string{"add", "--help"})
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(out)
	if err := root.Execute(); err != nil {
		t.Errorf("--help should not error: %v", err)
	}
	if !strings.Contains(out.String(), "Download a spec") {
		t.Errorf("--help output missing description, got: %s", out.String())
	}
}

func TestRootCmd_UnknownSubcommandErrors(t *testing.T) {
	t.Parallel()
	root := newRootCmd()
	root.SetArgs([]string{"nonexistent"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	if err := root.Execute(); err == nil {
		t.Error("expected error for unknown subcommand")
	}
}
