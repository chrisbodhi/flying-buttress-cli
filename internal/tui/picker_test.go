package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"buttress/internal/versions"
)

// keyMsg constructs a tea.KeyMsg for a given rune key. For special keys we use
// the type-keyed variants tea exposes.
func runeKey(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

var (
	keyUp    = tea.KeyMsg{Type: tea.KeyUp}
	keyDown  = tea.KeyMsg{Type: tea.KeyDown}
	keyEnter = tea.KeyMsg{Type: tea.KeyEnter}
	keyEsc   = tea.KeyMsg{Type: tea.KeyEsc}
	keyCtrlC = tea.KeyMsg{Type: tea.KeyCtrlC}
)

func sampleVersions() []versions.Version {
	return []versions.Version{
		{Hash: "sha256:aaa", Description: "first"},
		{Hash: "sha256:bbb", Description: "second"},
		{Hash: "sha256:ccc", Description: "third"},
	}
}

func TestPicker_InitNoCmd(t *testing.T) {
	t.Parallel()
	p := NewPicker("@org/pkg", sampleVersions())
	if cmd := p.Init(); cmd != nil {
		t.Errorf("Init() = %v, want nil", cmd)
	}
}

func TestPicker_NavigationDown(t *testing.T) {
	t.Parallel()
	p := NewPicker("pkg", sampleVersions())
	for _, msg := range []tea.Msg{keyDown, keyDown} {
		p.Update(msg)
	}
	if p.cursor != 2 {
		t.Errorf("cursor = %d, want 2", p.cursor)
	}
	// Going past the end should clamp.
	p.Update(keyDown)
	if p.cursor != 2 {
		t.Errorf("cursor clamped to %d, want 2", p.cursor)
	}
}

func TestPicker_NavigationUp(t *testing.T) {
	t.Parallel()
	p := NewPicker("pkg", sampleVersions())
	p.cursor = 2
	for _, msg := range []tea.Msg{keyUp, keyUp} {
		p.Update(msg)
	}
	if p.cursor != 0 {
		t.Errorf("cursor = %d, want 0", p.cursor)
	}
	// Going past the top should clamp.
	p.Update(keyUp)
	if p.cursor != 0 {
		t.Errorf("cursor clamped to %d, want 0", p.cursor)
	}
}

func TestPicker_VimKeybinds(t *testing.T) {
	t.Parallel()
	p := NewPicker("pkg", sampleVersions())
	p.Update(runeKey('j'))
	p.Update(runeKey('j'))
	if p.cursor != 2 {
		t.Errorf("after jj cursor = %d, want 2", p.cursor)
	}
	p.Update(runeKey('k'))
	if p.cursor != 1 {
		t.Errorf("after k cursor = %d, want 1", p.cursor)
	}
}

func TestPicker_EnterSelects(t *testing.T) {
	t.Parallel()
	p := NewPicker("pkg", sampleVersions())
	p.Update(keyDown)
	_, cmd := p.Update(keyEnter)
	if cmd == nil {
		t.Error("expected tea.Quit cmd on enter")
	}
	chosen := p.Chosen()
	if chosen == nil {
		t.Fatal("Chosen() = nil after enter")
	}
	if chosen.Hash != "sha256:bbb" {
		t.Errorf("Chosen().Hash = %q, want sha256:bbb", chosen.Hash)
	}
}

func TestPicker_SpaceSelects(t *testing.T) {
	t.Parallel()
	p := NewPicker("pkg", sampleVersions())
	_, cmd := p.Update(runeKey(' '))
	if cmd == nil {
		t.Error("expected tea.Quit cmd on space")
	}
	if p.Chosen() == nil {
		t.Error("Chosen() = nil after space")
	}
}

func TestPicker_QuittingKeys(t *testing.T) {
	t.Parallel()

	for _, msg := range []tea.Msg{keyEsc, keyCtrlC, runeKey('q')} {
		p := NewPicker("pkg", sampleVersions())
		_, cmd := p.Update(msg)
		if cmd == nil {
			t.Errorf("msg %v: expected tea.Quit cmd", msg)
		}
		if p.Chosen() != nil {
			t.Errorf("msg %v: Chosen() should be nil after quit", msg)
		}
		if !p.quitting {
			t.Errorf("msg %v: quitting should be true", msg)
		}
	}
}

func TestPicker_View_Default(t *testing.T) {
	t.Parallel()
	p := NewPicker("@org/pkg", sampleVersions())
	out := p.View()

	// Header mentions package name.
	if !strings.Contains(out, "@org/pkg") {
		t.Errorf("View() missing package name, got:\n%s", out)
	}
	// Each version description appears.
	for _, want := range []string{"first", "second", "third"} {
		if !strings.Contains(out, want) {
			t.Errorf("View() missing %q, got:\n%s", want, out)
		}
	}
	// Hint line exists.
	if !strings.Contains(out, "navigate") {
		t.Errorf("View() missing navigation hint, got:\n%s", out)
	}
}

func TestPicker_View_Quitting(t *testing.T) {
	t.Parallel()
	p := NewPicker("pkg", sampleVersions())
	p.quitting = true
	out := p.View()
	if !strings.Contains(out, "Cancelled") {
		t.Errorf("quitting view should say 'Cancelled', got:\n%s", out)
	}
}

func TestPicker_NonKeyMsgNoOp(t *testing.T) {
	t.Parallel()
	p := NewPicker("pkg", sampleVersions())
	// Send an unrelated msg type — should not change state.
	type bogus struct{}
	prev := p.cursor
	_, cmd := p.Update(bogus{})
	if cmd != nil {
		t.Errorf("non-key msg returned cmd %v", cmd)
	}
	if p.cursor != prev {
		t.Errorf("cursor changed on non-key msg")
	}
}

func TestNewPicker(t *testing.T) {
	t.Parallel()
	vs := sampleVersions()
	p := NewPicker("pkg", vs)
	if p.pkg != "pkg" {
		t.Errorf("pkg = %q", p.pkg)
	}
	if len(p.versions) != len(vs) {
		t.Errorf("versions len = %d, want %d", len(p.versions), len(vs))
	}
	if p.Chosen() != nil {
		t.Error("Chosen() should be nil on fresh Picker")
	}
}
