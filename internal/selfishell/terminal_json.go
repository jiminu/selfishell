package selfishell

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// Windows Terminal accepts JSON comments and trailing commas. Keep byte offsets
// so changing two properties never reformats unrelated settings or comments.
type terminalJSON struct {
	data, clean, punctuation []byte
	root                     *terminalJSONNode
}

type terminalJSONNode struct {
	start, end int
	members    []terminalJSONMember
	items      []*terminalJSONNode
	object     bool
}

type terminalJSONMember struct {
	key   string
	start int
	value *terminalJSONNode
}

func parseTerminalJSON(data []byte) (*terminalJSON, error) {
	clean := bytes.Clone(data)
	if bytes.HasPrefix(clean, []byte{0xef, 0xbb, 0xbf}) {
		copy(clean, "   ")
	}
	for i := 0; i < len(clean); i++ {
		if clean[i] == '"' {
			for i++; i < len(clean); i++ {
				if clean[i] == '\\' {
					i++
				} else if clean[i] == '"' {
					break
				}
			}
		} else if i+1 < len(clean) && clean[i] == '/' && (clean[i+1] == '/' || clean[i+1] == '*') {
			start, block := i, clean[i+1] == '*'
			i += 2
			if block {
				for i+1 < len(clean) && !(clean[i] == '*' && clean[i+1] == '/') {
					i++
				}
				if i+1 >= len(clean) {
					return nil, fmt.Errorf("unterminated Windows Terminal settings comment")
				}
				i += 2
			} else {
				for i < len(clean) && clean[i] != '\n' {
					i++
				}
			}
			for j := start; j < i; j++ {
				if clean[j] != '\n' && clean[j] != '\r' {
					clean[j] = ' '
				}
			}
			i--
		}
	}
	punctuation := bytes.Clone(clean)
	for i := 0; i < len(clean); i++ {
		if clean[i] == '"' {
			for i++; i < len(clean); i++ {
				if clean[i] == '\\' {
					i++
				} else if clean[i] == '"' {
					break
				}
			}
		} else if clean[i] == ',' {
			j := i + 1
			for j < len(clean) && terminalJSONSpace(clean[j]) {
				j++
			}
			if j < len(clean) && (clean[j] == '}' || clean[j] == ']') {
				clean[i] = ' '
			}
		}
	}
	if !json.Valid(clean) {
		return nil, fmt.Errorf("invalid Windows Terminal settings JSON")
	}
	d := json.NewDecoder(bytes.NewReader(clean))
	d.UseNumber()
	root, err := readTerminalJSONNode(d, clean)
	if err != nil {
		return nil, err
	}
	return &terminalJSON{data: data, clean: clean, punctuation: punctuation, root: root}, nil
}

func terminalJSONSpace(ch byte) bool { return ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' }

func terminalJSONStart(d *json.Decoder, data []byte) int {
	i := int(d.InputOffset())
	for i < len(data) && (terminalJSONSpace(data[i]) || data[i] == ',' || data[i] == ':') {
		i++
	}
	return i
}

func readTerminalJSONNode(d *json.Decoder, data []byte) (*terminalJSONNode, error) {
	n := &terminalJSONNode{start: terminalJSONStart(d, data)}
	token, err := d.Token()
	if err != nil {
		return nil, err
	}
	if delimiter, ok := token.(json.Delim); ok {
		n.object = delimiter == '{'
		seen := map[string]bool{}
		for d.More() {
			var key string
			start := terminalJSONStart(d, data)
			if n.object {
				token, err = d.Token()
				if err != nil {
					return nil, err
				}
				key = token.(string)
				if seen[key] {
					return nil, fmt.Errorf("duplicate Windows Terminal settings key: %s", key)
				}
				seen[key] = true
			}
			child, err := readTerminalJSONNode(d, data)
			if err != nil {
				return nil, err
			}
			if n.object {
				n.members = append(n.members, terminalJSONMember{key, start, child})
			} else {
				n.items = append(n.items, child)
			}
		}
		if _, err = d.Token(); err != nil {
			return nil, err
		}
	}
	n.end = int(d.InputOffset())
	return n, nil
}

func (n *terminalJSONNode) property(key string) *terminalJSONNode {
	if n != nil {
		for _, m := range n.members {
			if m.key == key {
				return m.value
			}
		}
	}
	return nil
}

func (j *terminalJSON) value(n *terminalJSONNode) json.RawMessage {
	if n == nil {
		return nil
	}
	var value bytes.Buffer
	_ = json.Compact(&value, j.clean[n.start:n.end])
	return value.Bytes()
}

func (j *terminalJSON) raw(n *terminalJSONNode) json.RawMessage {
	if n == nil {
		return nil
	}
	return j.data[n.start:n.end]
}

func (j *terminalJSON) text(n *terminalJSONNode) string {
	var value string
	_ = json.Unmarshal(j.value(n), &value)
	return value
}

type terminalJSONEdit struct {
	start, end int
	value      []byte
}

func (j *terminalJSON) set(n *terminalJSONNode, key string, value json.RawMessage, edits *[]terminalJSONEdit) {
	for i, m := range n.members {
		if m.key != key {
			continue
		}
		if value != nil {
			*edits = append(*edits, terminalJSONEdit{m.value.start, m.value.end, value})
			return
		}
		// Comments are whitespace in punctuation. Retain their original bytes
		// and line endings while removing the property's JSON tokens.
		var preserved []byte
		for p := m.start; p < m.value.end; p++ {
			if terminalJSONSpace(j.punctuation[p]) {
				preserved = append(preserved, j.data[p])
			}
		}
		*edits = append(*edits, terminalJSONEdit{m.start, m.value.end, preserved})
		// Remove only one separator; leave surrounding whitespace and comments.
		from, to := m.value.end, n.end-1
		if i+1 < len(n.members) {
			to = n.members[i+1].start
		} else if i > 0 {
			from, to = n.members[i-1].value.end, m.start
		}
		for p := from; p < to; p++ {
			if j.punctuation[p] == ',' {
				*edits = append(*edits, terminalJSONEdit{p, p + 1, nil})
				break
			}
		}
		return
	}
	if value != nil {
		pos, prefix := n.start+1, ""
		if len(n.members) > 0 {
			pos, prefix = n.members[len(n.members)-1].value.end, ","
		}
		name, _ := json.Marshal(key)
		content := append([]byte(prefix+" "+string(name)+": "), value...)
		*edits = append(*edits, terminalJSONEdit{pos, pos, content})
	}
}

func (j *terminalJSON) apply(edits []terminalJSONEdit) ([]byte, error) {
	sort.SliceStable(edits, func(a, b int) bool { return edits[a].start > edits[b].start })
	data := bytes.Clone(j.data)
	for _, e := range edits {
		data = append(append(append([]byte{}, data[:e.start]...), e.value...), data[e.end:]...)
	}
	if _, err := parseTerminalJSON(data); err != nil {
		return nil, err
	}
	return data, nil
}
