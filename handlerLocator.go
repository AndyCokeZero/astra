package astra

import "strings"

// HandlerLocator is an interface for locating handler function source positions.
// It is used by input modules (e.g., gin) to find the file and line number of handler functions.
type HandlerLocator interface {
	// Locate returns the file path and line number for a handler by its runtime name.
	// The name is typically obtained from runtime.FuncForPC.
	// Returns ok=false if the handler is not found.
	Locate(handlerName string) (file string, line int, ok bool)
}

// HandlerLocation represents a handler's source location.
type HandlerLocation struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

// MapHandlerLocator is a map-based implementation of HandlerLocator.
// Keys should match the format used by runtime.FuncForPC (e.g., "main.GetUser" or "main.(*Controller).GetUser").
type MapHandlerLocator map[string]HandlerLocation

// Locate finds a handler location by name.
// It first tries an exact match, then tries without the "-fm" suffix (used for bound methods).
func (m MapHandlerLocator) Locate(name string) (string, int, bool) {
	if m == nil {
		return "", 0, false
	}

	// Try exact match first
	if loc, ok := m[name]; ok {
		return loc.File, loc.Line, true
	}

	// Try without -fm suffix (bound method indicator)
	normalized := strings.TrimSuffix(name, "-fm")
	if normalized != name {
		if loc, ok := m[normalized]; ok {
			return loc.File, loc.Line, true
		}
	}

	return "", 0, false
}
