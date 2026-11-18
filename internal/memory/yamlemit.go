package memory

import (
	"bytes"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	tagInt   = "!!int"
	tagFloat = "!!float"
	tagBool  = "!!bool"
	// yamlIndent is the indent re-emitted frontmatter uses (D26: layout,
	// indentation included, is not guaranteed).
	yamlIndent = 2
)

// encodeYAML renders a YAML node as yaml.v3 writes it, LF line endings.
func encodeYAML(n *yaml.Node) (string, error) {
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(yamlIndent)
	if err := enc.Encode(n); err != nil {
		return "", err
	}
	if err := enc.Close(); err != nil {
		return "", err
	}
	return b.String(), nil
}

// strNode returns a string scalar that decodes back as exactly s. yaml.v3
// writes a !!str plain when it believes the plain form resolves as a
// string, but it is not always right - it writes "<<" plain, which reads
// back as a merge key - so the plain form is tried and, unless it reads
// back as the same string, the scalar is double-quoted.
func strNode(s string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.ScalarNode, Tag: tagStr, Value: s}
	if !scalarRoundTrips(n) {
		n.Style = yaml.DoubleQuotedStyle
	}
	return n
}

// scalarRoundTrips reports whether n, written as a mapping value, reads
// back with the same tag and value.
func scalarRoundTrips(n *yaml.Node) bool {
	key := &yaml.Node{Kind: yaml.ScalarNode, Tag: tagStr, Value: "k"}
	text, err := encodeYAML(&yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{key, n}})
	if err != nil {
		return false
	}
	var doc yaml.Node
	if yaml.Unmarshal([]byte(text), &doc) != nil || len(doc.Content) != 1 || len(doc.Content[0].Content) != 2 {
		return false
	}
	got := doc.Content[0].Content[1]
	return got.Kind == yaml.ScalarNode && got.Tag == n.Tag && got.Value == n.Value
}

// valueNode renders a normalised property value (see propertyValueIn).
func valueNode(v any) *yaml.Node {
	switch x := v.(type) {
	case string:
		return strNode(x)
	case bool:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: tagBool, Value: strconv.FormatBool(x)}
	case int64:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: tagInt, Value: strconv.FormatInt(x, 10)}
	case float64:
		return floatNode(x)
	case []string:
		return listNode(x)
	default: // nil
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: tagNull}
	}
}

// floatNode writes the shortest form that reads back as f. An integral
// float is written as an integer (JSON cannot tell 3.0 from 3; D27's known
// limit), anything else in a form YAML resolves as a float.
func floatNode(f float64) *yaml.Node {
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: tagInt, Value: s}
	}
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: tagFloat, Value: s}
}

// listNode renders a list of text as a flow sequence: [a, b].
func listNode(items []string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.SequenceNode, Tag: tagSeq, Style: yaml.FlowStyle}
	for _, s := range items {
		n.Content = append(n.Content, strNode(s))
	}
	return n
}

// copyYAML deep-copies a YAML tree. Aliases in the copy point at the
// copy's own anchored nodes, so the original is never reachable from it.
func copyYAML(n *yaml.Node) *yaml.Node {
	copies := map[*yaml.Node]*yaml.Node{}
	out := copyNode(n, copies)
	relinkAliases(out, copies)
	return out
}

func copyNode(n *yaml.Node, copies map[*yaml.Node]*yaml.Node) *yaml.Node {
	if n == nil {
		return nil
	}
	c := *n
	c.Content = make([]*yaml.Node, len(n.Content))
	for i, child := range n.Content {
		c.Content[i] = copyNode(child, copies)
	}
	copies[n] = &c
	return &c
}

// relinkAliases points every alias in the copied tree at the copy of its
// target; an alias whose target lies outside the tree keeps a copy of it.
func relinkAliases(n *yaml.Node, copies map[*yaml.Node]*yaml.Node) {
	walkYAML(n, func(x *yaml.Node) {
		if x.Kind != yaml.AliasNode || x.Alias == nil {
			return
		}
		if c, ok := copies[x.Alias]; ok {
			x.Alias = c
			return
		}
		x.Alias = copyNode(x.Alias, copies)
	})
}

// walkYAML visits n and every node beneath it, never following an alias
// to its target.
func walkYAML(n *yaml.Node, visit func(*yaml.Node)) {
	if n == nil {
		return
	}
	visit(n)
	for _, c := range n.Content {
		walkYAML(c, visit)
	}
}

// replaceValue replaces the value at index i of mapping m (a deep copy) by
// v. Aliases elsewhere in root that point into the old value are first
// expanded to copies of what they pointed at, so every untouched field
// keeps its value and the YAML stays valid (D26). The new value takes the
// old one's comments but not its anchor (D27).
func replaceValue(root, m *yaml.Node, i int, v *yaml.Node) {
	old := m.Content[i]
	anchored := map[*yaml.Node]bool{}
	walkYAML(old, func(x *yaml.Node) {
		if x.Anchor != "" {
			anchored[x] = true
		}
	})
	if len(anchored) > 0 {
		expandAliases(root, old, anchored)
	}
	v.HeadComment, v.LineComment, v.FootComment = old.HeadComment, old.LineComment, old.FootComment
	m.Content[i] = v
}

// expandAliases replaces, everywhere in root outside skip, each alias to a
// node in targets by a copy of that node without its anchor, keeping the
// alias's own comments.
func expandAliases(root, skip *yaml.Node, targets map[*yaml.Node]bool) {
	var visit func(n *yaml.Node)
	visit = func(n *yaml.Node) {
		if n == skip {
			return
		}
		for i, c := range n.Content {
			if c.Kind == yaml.AliasNode && targets[c.Alias] {
				n.Content[i] = expandedCopy(c)
				continue
			}
			visit(c)
		}
	}
	visit(root)
}

func expandedCopy(alias *yaml.Node) *yaml.Node {
	c := copyYAML(alias.Alias)
	c.Anchor = ""
	c.HeadComment, c.LineComment, c.FootComment = alias.HeadComment, alias.LineComment, alias.FootComment
	return c
}
