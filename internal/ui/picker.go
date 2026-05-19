package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Picker is a generic state manager for dropdown lists in the UI.
// It manages cursor bounds, scrolling offsets, and filtering of items.
type Picker[T any] struct {
	Items    []T
	Filtered []T
	Cursor   int
	Offset   int

	FilterFn  func(item T, queryWords []string) bool
}

// NewPicker creates a new Picker with the given items and filter function.
func NewPicker[T any](items []T, filterFn func(T, []string) bool) *Picker[T] {
	return &Picker[T]{
		Items:    items,
		Filtered: items,
		FilterFn: filterFn,
	}
}

// SetItems replaces the picker's items and resets the filter to show all items.
func (p *Picker[T]) SetItems(items []T) {
	p.Items = items
	p.Filtered = items
	p.Cursor = 0
	p.Offset = 0
}

// Filter applies the filter function against the current query string.
func (p *Picker[T]) Filter(query string) {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		p.Filtered = p.Items
		p.Cursor = 0
		p.Offset = 0
		return
	}
	words := strings.Fields(query)
	var result []T
	for _, item := range p.Items {
		if p.FilterFn(item, words) {
			result = append(result, item)
		}
	}
	p.Filtered = result
	if p.Cursor >= len(p.Filtered) {
		p.Cursor = max(0, len(p.Filtered)-1)
	}
	if p.Offset > p.Cursor {
		p.Offset = p.Cursor
	}
}

// MoveCursor adjusts the cursor position by delta, clamping it to the filtered list bounds.
func (p *Picker[T]) MoveCursor(delta int) {
	p.Cursor += delta
	p.Cursor = clamp(p.Cursor, 0, max(0, len(p.Filtered)-1))
}

// HandleKey processes standard navigation keys. Returns true if the key was handled.
func (p *Picker[T]) HandleKey(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "up", "ctrl+p":
		p.MoveCursor(-1)
		return true
	case "down", "ctrl+n":
		p.MoveCursor(1)
		return true
	case "pgup":
		p.MoveCursor(-10)
		return true
	case "pgdown":
		p.MoveCursor(10)
		return true
	}
	return false
}

// Selected returns the currently selected item, and a boolean indicating if one exists.
func (p *Picker[T]) Selected() (T, bool) {
	var zero T
	if p.Cursor < 0 || p.Cursor >= len(p.Filtered) {
		return zero, false
	}
	return p.Filtered[p.Cursor], true
}
