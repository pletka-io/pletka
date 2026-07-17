package scope

import (
	"net/url"
	"strings"
)

// Scope builds URLs relative to a mounted schema UI surface.
type Scope struct {
	Prefix string
}

func New(prefix string) Scope {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" || prefix == "/" {
		return Scope{Prefix: ""}
	}
	return Scope{Prefix: "/" + strings.Trim(prefix, "/")}
}

func Project(projectID string) Scope {
	return New("/projects/" + url.PathEscape(projectID))
}

func (s Scope) URL(parts ...string) string {
	segments := make([]string, 0, len(parts)+1)
	if prefix := strings.Trim(s.Prefix, "/"); prefix != "" {
		segments = append(segments, splitPath(prefix)...)
	}
	for _, part := range parts {
		segments = append(segments, splitPath(part)...)
	}
	if len(segments) == 0 {
		return "/"
	}
	return "/" + strings.Join(segments, "/")
}

func (s Scope) Child(parts ...string) Scope {
	return New(s.URL(parts...))
}

func splitPath(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" || value == "/" {
		return nil
	}
	raw := strings.Split(strings.Trim(value, "/"), "/")
	segments := raw[:0]
	for _, segment := range raw {
		if segment == "" {
			continue
		}
		segments = append(segments, segment)
	}
	return segments
}
