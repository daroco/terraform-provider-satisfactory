package export

import (
	"fmt"

	"github.com/daroco/terraform-provider-satisfactory/internal/api"
)

// Level ranks a Finding.
type Level int

const (
	// LevelOK records something that was checked and looked right. Worth
	// printing: "no problems found" is much less useful than "foundations are
	// being reported", because the reader learns which failure was ruled out.
	LevelOK Level = iota
	// LevelWarn is suspicious but legitimately possible.
	LevelWarn
	// LevelFail is a response that cannot be correct.
	LevelFail
)

func (l Level) String() string {
	switch l {
	case LevelFail:
		return "FAIL"
	case LevelWarn:
		return "warn"
	default:
		return "ok"
	}
}

// Finding is one observation about a world listing.
type Finding struct {
	Level   Level
	Message string
	// Detail explains what the finding means for someone who did not write
	// the checker, and is printed underneath the message.
	Detail string
}

// Check inspects what the mod reported about a region and calls out anything
// that cannot be right.
//
// It exists because the mock cannot answer the question that matters. The mock
// has no lightweight buildables, no real connector components and no actor
// list - precisely the things export has to get right - so a green test suite
// says nothing about whether these endpoints work in a game. This runs against
// the live mod instead, and knows what a real factory must look like.
func Check(items []api.WorldBuildable, players []api.Player) []Finding {
	var out []Finding
	add := func(l Level, msg, detail string) {
		out = append(out, Finding{Level: l, Message: msg, Detail: detail})
	}

	if len(items) == 0 {
		add(LevelFail, "nothing found in range at all",
			"Either the coordinates are wrong, or enumeration is returning an empty world.")
		return out
	}

	var lightweight, tracked, connectionClasses, resolved int
	for _, it := range items {
		if it.Lightweight {
			lightweight++
		}
		if it.TFID != "" {
			tracked++
		}
		if isConnectionClass(it.Class) {
			connectionClasses++
			if it.Connects != nil {
				resolved++
			}
		}
	}

	add(LevelOK, fmt.Sprintf("%d buildables: %d actors, %d lightweight; %d tracked by Terraform, %d built in-game",
		len(items), len(items)-lightweight, lightweight, tracked, len(items)-tracked), "")

	// The failure this endpoint exists to avoid. Foundations are lightweight
	// instances rather than actors, so an implementation built on the actor
	// list alone returns a factory with no floor - and looks healthy doing it.
	if lightweight == 0 {
		add(LevelWarn, "no lightweight buildables reported",
			"Correct only if there is genuinely no foundation, wall or ramp in range. "+
				"Stand on a floor and re-run: if it is still zero, the lightweight half of "+
				"the union is broken and every export will be missing its floor.")
	} else {
		add(LevelOK, fmt.Sprintf("%d lightweight instances reported", lightweight),
			"Foundations are not actors; seeing them means both enumeration sources are live.")
	}

	switch {
	case connectionClasses == 0:
		add(LevelWarn, "no belts or wires in range",
			"The connection graph is untested by this run. Re-run somewhere with belts.")
	case resolved == 0:
		add(LevelFail, fmt.Sprintf("%d belts/wires in range, none resolved their endpoints", connectionClasses),
			"Either connector lookup or the two-pass index mapping is broken. Every "+
				"exported factory would come out unwired.")
	default:
		add(LevelOK, fmt.Sprintf("%d of %d belts/wires resolved both ends", resolved, connectionClasses),
			"Unresolved ones are expected at the edge of the radius, where the far end is out of range.")
	}

	// Indices are the only handle an untracked buildable has, so every
	// endpoint must address something in this same response.
	badIndex, selfRef := 0, 0
	for _, it := range items {
		if it.Connects == nil {
			continue
		}
		for _, e := range []api.WorldEndpoint{it.Connects.From, it.Connects.To} {
			if e.Index < 0 || int(e.Index) >= len(items) {
				badIndex++
			}
		}
		if it.Connects.From.Index == it.Connects.To.Index {
			selfRef++
		}
	}
	if badIndex > 0 {
		add(LevelFail, fmt.Sprintf("%d connection endpoint(s) point outside the response", badIndex),
			"Index mapping is wrong. Re-created, these would wire whatever happens to sit at that index.")
	} else if resolved > 0 {
		add(LevelOK, "every connection endpoint indexes a buildable in this response", "")
	}
	if selfRef > 0 {
		add(LevelFail, fmt.Sprintf("%d connection(s) join a buildable to itself", selfRef), "")
	}

	// Duplicate indices would break every reference in the generated file.
	seen := map[int64]bool{}
	for _, it := range items {
		if seen[it.Index] {
			add(LevelFail, fmt.Sprintf("index %d appears more than once", it.Index),
				"Indices must be unique within a response; connections reference them.")
			break
		}
		seen[it.Index] = true
	}

	if len(players) == 0 {
		add(LevelWarn, "no players in the world", "-at-player has nothing to anchor to.")
	}

	return out
}

// Failed reports whether any finding is fatal.
func Failed(findings []Finding) bool {
	for _, f := range findings {
		if f.Level == LevelFail {
			return true
		}
	}
	return false
}
