package jira

import (
	"fmt"
	"strings"
)

// ResolveTransition maps a user-supplied transition selector (its id, or its
// name typed however the user cased it) to the concrete Transition to POST.
// Jira's transition endpoint takes a transition id, but ids are opaque
// per-workflow numbers, so users name the destination instead ("Done",
// "In Progress"); this resolves that against the issue's currently available
// transitions. An exact id match wins first; otherwise names are matched
// case-insensitively. A selector matching nothing, or ambiguously matching
// several names, is a caller-visible error rather than a silent guess.
func ResolveTransition(transitions []Transition, selector string) (Transition, error) {
	for _, t := range transitions {
		if t.ID == selector {
			return t, nil
		}
	}

	var matches []Transition
	for _, t := range transitions {
		if strings.EqualFold(t.Name, selector) {
			matches = append(matches, t)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return Transition{}, fmt.Errorf("no transition matches %q; available: %s", selector, transitionNames(transitions))
	default:
		return Transition{}, fmt.Errorf("transition %q is ambiguous (%d matches by name)", selector, len(matches))
	}
}

// transitionNames lists the available transition names for an error hint.
func transitionNames(transitions []Transition) string {
	names := make([]string, 0, len(transitions))
	for _, t := range transitions {
		names = append(names, t.Name)
	}
	if len(names) == 0 {
		return "(none)"
	}
	return strings.Join(names, ", ")
}
