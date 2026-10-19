package model

import (
	"fmt"
	"strconv"
	"strings"
)

// LayoutHintsVersion is the carrier format this build emits and accepts.
const LayoutHintsVersion = 1

// ScopeHints holds one scope's placement decisions: the layer index and
// in-layer index of every id laid out in that scope's own run (member
// nodes and, as macro nodes, direct child groups).
type ScopeHints struct {
	Layers map[string]int `json:"layers"`
	Order  map[string]int `json:"order"`
}

// LayoutHints is the anchoring carrier (C18): a unit-free record of a
// layout's layers, in-layer order, cycle-break decisions and flat-edge
// fallbacks, keyed by scope (a group id, or "" for the top level). It is
// emitted with every flow and class layout, embedded in SVG output as
// <metadata id="diago-layout">, and accepted back as a "previous" layout.
// It carries ids only, never labels.
type LayoutHints struct {
	Version  int                   `json:"version"`
	Scopes   map[string]ScopeHints `json:"scopes"`
	Reversed []string              `json:"reversed"`         // logical edge ids, sorted
	Ranked   []string              `json:"ranked,omitempty"` // the flat edges that fell back to ordinary ones (S9), logical ids, sorted; absent when none
}

// Slot is a carrier key for edge id's vertex on layer l of a scope where it
// has no node of its own: a dummy, or a terminal on the scope's border row
// (C18).
func Slot(id string, l int) string { return id + "@" + strconv.Itoa(l) }

// SplitSlot splits a slot key into its edge id and layer; ok is false for
// any other key, a node's or a group's id among them, since those never end
// in "@" and digits.
func SplitSlot(key string) (id string, l int, ok bool) {
	i := strings.LastIndexByte(key, '@')
	if i < 0 || i == len(key)-1 {
		return "", 0, false
	}
	for _, r := range key[i+1:] {
		if r < '0' || r > '9' {
			return "", 0, false
		}
	}
	l, err := strconv.Atoi(key[i+1:])
	if err != nil {
		return "", 0, false
	}
	return key[:i], l, true
}

// NewLayoutHints returns an empty carrier of the current version.
func NewLayoutHints() *LayoutHints {
	return &LayoutHints{Version: LayoutHintsVersion, Scopes: map[string]ScopeHints{}, Reversed: []string{}}
}

// Scope returns the hints for scope id, creating it (and any nil map) on
// first use. The returned maps are the stored maps.
func (h *LayoutHints) Scope(id string) ScopeHints {
	if h.Scopes == nil {
		h.Scopes = map[string]ScopeHints{}
	}
	s := h.Scopes[id]
	if s.Layers == nil {
		s.Layers = map[string]int{}
	}
	if s.Order == nil {
		s.Order = map[string]int{}
	}
	h.Scopes[id] = s
	return s
}

// Validate checks the carrier's shape: the supported version and a scopes
// map. A nil Reversed is tolerated (an absent list means no reversals).
func (h *LayoutHints) Validate() error {
	if h == nil {
		return fmt.Errorf("layout hints: nil")
	}
	if h.Version != LayoutHintsVersion {
		return fmt.Errorf("layout hints: unsupported version %d (want %d)", h.Version, LayoutHintsVersion)
	}
	if h.Scopes == nil {
		return fmt.Errorf("layout hints: missing scopes")
	}
	return nil
}
