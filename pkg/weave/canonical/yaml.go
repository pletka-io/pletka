package canonical

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

// Encode serializes v to canonical YAML with sorted map keys at every level.
// Output uses indent 2 and ends with a single newline.
func Encode(v any) ([]byte, error) {
	normalized := normalize(v)

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)

	if err := enc.Encode(normalized); err != nil {
		return nil, fmt.Errorf("canonical encode: %w", err)
	}

	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("canonical encode close: %w", err)
	}

	return buf.Bytes(), nil
}

// Hash returns the SHA-256 hex digest of payload.
func Hash(payload []byte) string {
	h := sha256.Sum256(payload)
	return fmt.Sprintf("%x", h[:])
}

// normalize recursively rewrites maps into *yaml.Node MappingNodes with
// alphabetically sorted keys so encoding is deterministic.
func normalize(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return sortedMappingNode(val)
	case map[string]string:
		m := make(map[string]any, len(val))
		for k, v := range val {
			m[k] = v
		}
		return sortedMappingNode(m)
	case []any:
		result := make([]any, len(val))
		for i, item := range val {
			result[i] = normalize(item)
		}
		return result
	default:
		return v
	}
}

// sortedMappingNode builds a yaml.Node of kind MappingNode with keys in
// alphabetical order. Values are recursively normalized.
func sortedMappingNode(m map[string]any) *yaml.Node {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	node := &yaml.Node{
		Kind: yaml.MappingNode,
	}

	for _, k := range keys {
		keyNode := &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: k,
		}
		valNode := valueToNode(normalize(m[k]))
		node.Content = append(node.Content, keyNode, valNode)
	}

	return node
}

// valueToNode converts an arbitrary Go value into a *yaml.Node.
func valueToNode(v any) *yaml.Node {
	switch val := v.(type) {
	case *yaml.Node:
		return val
	case nil:
		return &yaml.Node{
			Kind:  yaml.ScalarNode,
			Tag:   "!!null",
			Value: "null",
		}
	case []any:
		seq := &yaml.Node{Kind: yaml.SequenceNode}
		for _, item := range val {
			seq.Content = append(seq.Content, valueToNode(normalize(item)))
		}
		return seq
	default:
		var node yaml.Node
		if err := node.Encode(val); err != nil {
			// Fallback: encode as string.
			return &yaml.Node{
				Kind:  yaml.ScalarNode,
				Value: fmt.Sprintf("%v", val),
			}
		}
		return &node
	}
}
