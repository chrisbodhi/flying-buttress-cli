package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"buttress/internal/versions"
)

// Picker is a Bubble Tea model that lets the user select a spec version from
// a list of SHAs and descriptions.
type Picker struct {
	pkg      string
	versions []versions.Version
	cursor   int
	chosen   *versions.Version
	quitting bool
}

// NewPicker creates a Picker for the given package and version list.
func NewPicker(pkg string, vs []versions.Version) *Picker {
	return &Picker{pkg: pkg, versions: vs}
}

// Chosen returns the version selected by the user, or nil if the user quit
// without making a selection.
func (p *Picker) Chosen() *versions.Version {
	return p.chosen
}

func (p *Picker) Init() tea.Cmd {
	return nil
}

func (p *Picker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			p.quitting = true
			return p, tea.Quit
		case "up", "k":
			if p.cursor > 0 {
				p.cursor--
			}
		case "down", "j":
			if p.cursor < len(p.versions)-1 {
				p.cursor++
			}
		case "enter", " ":
			v := p.versions[p.cursor]
			p.chosen = &v
			return p, tea.Quit
		}
	}
	return p, nil
}

func (p *Picker) View() string {
	if p.quitting {
		return StyleDim.Render("Cancelled.") + "\n"
	}

	header := StyleTitle.Render(fmt.Sprintf("Select a version of %s", p.pkg)) + "\n"
	hint := StyleDim.Render("↑/↓ navigate · enter select · q quit") + "\n\n"

	var rows string
	for i, v := range p.versions {
		c := noCursor
		sha := StyleSHA.Render(ShortHash(v.Hash))
		desc := StyleDesc.Render(v.Description)

		if i == p.cursor {
			c = cursor
			sha = StyleSelected.Width(10).Render(ShortHash(v.Hash))
			desc = StyleSelected.Render(v.Description)
		}

		rows += fmt.Sprintf("%s %s  %s\n", c, sha, desc)
	}

	return header + hint + rows
}

// RunPicker runs the interactive version picker and returns the chosen version.
// Returns nil if the user quit without selecting.
func RunPicker(pkg string, vs []versions.Version) (*versions.Version, error) {
	m := NewPicker(pkg, vs)
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("version picker: %w", err)
	}
	return final.(*Picker).Chosen(), nil
}
