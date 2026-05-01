package generate

import (
	"context"
	"errors"
	"testing"
)

func TestStub_GenerateReturnsErrNotConfigured(t *testing.T) {
	t.Parallel()

	g := Stub{}
	err := g.Generate(context.Background(), Request{
		Spec:        &SpecArchive{Dir: "/x", ContentHash: "sha256:abc"},
		Language:    "go",
		OutputPath:  "/out/file.go",
		PackageName: "@org/pkg",
		ProjectDir:  "/proj",
	})

	if !errors.Is(err, ErrNotConfigured) {
		t.Errorf("Generate() = %v, want errors.Is ErrNotConfigured", err)
	}
}

// TestStub_ImplementsGenerator pins down the interface contract: Stub must be
// a Generator. If this stops compiling, the interface or Stub has drifted.
func TestStub_ImplementsGenerator(t *testing.T) {
	t.Parallel()
	var _ Generator = Stub{}
}

func TestSpecArchive_FieldsAccessible(t *testing.T) {
	t.Parallel()
	a := SpecArchive{Dir: "/tmp/x", ContentHash: "sha256:abc"}
	if a.Dir != "/tmp/x" {
		t.Errorf("Dir = %q", a.Dir)
	}
	if a.ContentHash != "sha256:abc" {
		t.Errorf("ContentHash = %q", a.ContentHash)
	}
}

func TestRequest_FieldsAccessible(t *testing.T) {
	t.Parallel()
	r := Request{
		Spec:        &SpecArchive{Dir: "/d"},
		Language:    "ts",
		OutputPath:  "/o.ts",
		PackageName: "@a/b",
		ProjectDir:  "/p",
		MaxAttempts: 5,
	}
	if r.Spec.Dir != "/d" || r.Language != "ts" || r.OutputPath != "/o.ts" {
		t.Errorf("Request fields not accessible: %+v", r)
	}
	if r.PackageName != "@a/b" || r.ProjectDir != "/p" || r.MaxAttempts != 5 {
		t.Errorf("Request fields not accessible: %+v", r)
	}
}
