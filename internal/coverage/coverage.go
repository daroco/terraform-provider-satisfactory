// Package coverage turns the mod's class catalog into an answer to "how much
// of the game can Terraform place?" - as a number, and as a list of exactly
// what is missing and why.
//
// It exists because "everything placeable" is a goal that cannot be tracked by
// feel. The catalog is read from the game, so the report cannot drift from
// what the game actually ships; and each missing class names the mechanism it
// needs, so the gaps group into a handful of engineering problems rather than
// a hundred resource types.
package coverage

import (
	"fmt"
	"sort"
	"strings"

	"github.com/daroco/terraform-provider-satisfactory/internal/api"
)

// Group is every class sharing one mechanism.
type Group struct {
	Mechanism string
	Resource  string // Terraform resource that places these, or "" if none
	Why       string // for unsupported groups: what the mechanism needs
	Classes   []api.ClassInfo
}

// Summary is the catalog, aggregated.
type Summary struct {
	Total       int
	Supported   int
	Groups      []Group         // sorted: supported first, then largest gap first
	NoSettings  []api.ClassInfo // placeable, but with state the contract cannot express
	Unsupported int
}

// Percent is the headline number.
func (s Summary) Percent() float64 {
	if s.Total == 0 {
		return 0
	}
	return 100 * float64(s.Supported) / float64(s.Total)
}

// Summarize groups the catalog by mechanism.
func Summarize(classes []api.ClassInfo) Summary {
	var s Summary
	byMech := map[string]*Group{}
	for _, c := range classes {
		s.Total++
		if c.Supported {
			s.Supported++
		} else {
			s.Unsupported++
		}
		if c.SettingsNotModelled != "" {
			s.NoSettings = append(s.NoSettings, c)
		}
		g, ok := byMech[c.Mechanism]
		if !ok {
			g = &Group{Mechanism: c.Mechanism, Resource: c.Resource, Why: c.WhyUnsupported}
			byMech[c.Mechanism] = g
		}
		g.Classes = append(g.Classes, c)
	}
	for _, g := range byMech {
		sort.Slice(g.Classes, func(i, j int) bool { return g.Classes[i].Class < g.Classes[j].Class })
		s.Groups = append(s.Groups, *g)
	}
	sort.Slice(s.Groups, func(i, j int) bool {
		a, b := s.Groups[i], s.Groups[j]
		if (a.Resource != "") != (b.Resource != "") {
			return a.Resource != "" // supported groups first
		}
		if len(a.Classes) != len(b.Classes) {
			return len(a.Classes) > len(b.Classes) // then biggest gap first
		}
		return a.Mechanism < b.Mechanism
	})
	sort.Slice(s.NoSettings, func(i, j int) bool { return s.NoSettings[i].Class < s.NoSettings[j].Class })
	return s
}

// Report renders a summary for a terminal. listAll includes every class name
// under each group; otherwise supported groups show counts only, since the
// gaps are what anyone reading this wants to see.
func Report(s Summary, listAll bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d placeable classes in the game; the provider can place %d (%.0f%%).\n",
		s.Total, s.Supported, s.Percent())

	b.WriteString("\nPlaceable today\n")
	for _, g := range s.Groups {
		if g.Resource == "" {
			continue
		}
		fmt.Fprintf(&b, "  %-12s %4d  via %s\n", g.Mechanism, len(g.Classes), g.Resource)
		if listAll {
			for _, c := range g.Classes {
				fmt.Fprintf(&b, "               %s\n", c.Class)
			}
		}
	}

	if s.Unsupported > 0 {
		fmt.Fprintf(&b, "\nNot yet: %d classes, in %d mechanisms\n", s.Unsupported, countUnsupported(s.Groups))
		for _, g := range s.Groups {
			if g.Resource != "" {
				continue
			}
			fmt.Fprintf(&b, "  %-12s %4d  %s\n", g.Mechanism, len(g.Classes), g.Why)
			for _, c := range g.Classes {
				fmt.Fprintf(&b, "               %s\n", c.Class)
			}
		}
	}

	if len(s.NoSettings) > 0 {
		b.WriteString("\nPlaceable, but Terraform cannot configure them - it can put these down,\n")
		b.WriteString("it cannot make them do anything:\n")
		for _, c := range s.NoSettings {
			fmt.Fprintf(&b, "  %-40s %s\n", c.Class, c.SettingsNotModelled)
		}
	}
	return b.String()
}

func countUnsupported(groups []Group) int {
	n := 0
	for _, g := range groups {
		if g.Resource == "" {
			n++
		}
	}
	return n
}
