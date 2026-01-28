package utils

import "strings"

type HandlerPath struct {
	PathParts    []string
	HandlerParts []string
}

func SplitHandlerPath(handlerPath string) HandlerPath {
	// Create path parts by splitting on slashes
	pathParts := strings.Split(handlerPath, "/")

	// Create handler parts by splitting on dots from the last path part
	handlerParts := strings.Split(pathParts[len(pathParts)-1], ".")

	// Remove the last dot-separated part from the path parts
	pathParts = pathParts[:len(pathParts)-1]
	pathParts = append(pathParts, handlerParts[0])

	// Remove the first handler part from the handler parts
	handlerParts = handlerParts[1:]

	return HandlerPath{
		PathParts:    pathParts,
		HandlerParts: handlerParts,
	}
}

func (h HandlerPath) PackagePath() string {
	return strings.Join(h.PathParts, "/")
}

func (h HandlerPath) PackageName() string {
	return h.PathParts[len(h.PathParts)-1]
}

func (h HandlerPath) Handler() string {
	return strings.Join(h.HandlerParts, ".")
}

// FuncName returns the function name, handling both regular functions and methods.
// For regular functions like "main.GetPosts", it returns "GetPosts".
// For methods like "main.(*APIServer).GetContacts-fm", it returns "GetContacts".
func (h HandlerPath) FuncName() string {
	if len(h.HandlerParts) == 0 {
		return ""
	}

	// For method-style handlers, the actual method name is the last part
	// e.g., ["(*APIServer)", "GetContacts-fm"] -> "GetContacts"
	funcName := h.HandlerParts[len(h.HandlerParts)-1]

	// Remove the -fm suffix if present
	funcName = strings.TrimSuffix(funcName, "-fm")

	return funcName
}

// IsMethod returns true if this is a method-style handler (has a receiver).
// e.g., "main.(*APIServer).GetContacts-fm" is a method
func (h HandlerPath) IsMethod() bool {
	if len(h.HandlerParts) < 2 {
		return false
	}
	// Check if the first handler part looks like a receiver type
	first := h.HandlerParts[0]
	return strings.HasPrefix(first, "(") && strings.HasSuffix(first, ")")
}

// ReceiverType returns the receiver type name for method-style handlers.
// e.g., "main.(*APIServer).GetContacts-fm" -> "*APIServer"
// Returns empty string for regular functions.
func (h HandlerPath) ReceiverType() string {
	if !h.IsMethod() {
		return ""
	}
	recv := h.HandlerParts[0]
	// Remove surrounding parentheses
	recv = strings.TrimPrefix(recv, "(")
	recv = strings.TrimSuffix(recv, ")")
	return recv
}

// ReceiverTypeName returns the receiver type name without the pointer asterisk.
// e.g., "main.(*APIServer).GetContacts-fm" -> "APIServer"
func (h HandlerPath) ReceiverTypeName() string {
	recv := h.ReceiverType()
	return strings.TrimPrefix(recv, "*")
}
