package cmd

import (
	"strings"
	"testing"
)

func TestNewAuthCmd_Metadata(t *testing.T) {
	t.Parallel()
	c := newAuthCmd()
	if !strings.HasPrefix(c.Use, "auth") {
		t.Errorf("Use = %q, want prefix 'auth'", c.Use)
	}
	if c.Short == "" {
		t.Error("Short is empty")
	}
}

func TestNewAuthCmd_RunE_IsNoOp(t *testing.T) {
	t.Parallel()
	c := newAuthCmd()
	if err := c.RunE(c, nil); err != nil {
		t.Errorf("RunE() = %v, want nil", err)
	}
}
