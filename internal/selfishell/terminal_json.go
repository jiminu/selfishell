package selfishell

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Windows Terminal accepts JSON comments and trailing commas; Selfishell only reads it.
type terminalJSON struct {
	clean []byte
	root  *terminalJSONNode
}

type terminalJSONNode struct {
	start, end int
	members    []terminalJSONMember
	items      []*terminalJSONNode
	object     bool
}

type terminalJSONMember struct {
	key   string
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
	return &terminalJSON{clean: clean, root: root}, nil
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
		for d.More() {
			var key string
			if n.object {
				token, err = d.Token()
				if err != nil {
					return nil, err
				}
				key = token.(string)
			}
			child, err := readTerminalJSONNode(d, data)
			if err != nil {
				return nil, err
			}
			if n.object {
				n.members = append(n.members, terminalJSONMember{key, child})
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

// Windows Terminal accepts duplicate keys. Reject them only where Selfishell
// reads, since the first and last occurrence may disagree.
func (n *terminalJSONNode) unique(keys ...string) error {
	if n == nil {
		return nil
	}
	for _, key := range keys {
		count := 0
		for _, m := range n.members {
			if m.key == key {
				count++
			}
		}
		if count > 1 {
			return fmt.Errorf("duplicate Windows Terminal settings key: %s", key)
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

func (j *terminalJSON) text(n *terminalJSONNode) string {
	var value string
	_ = json.Unmarshal(j.value(n), &value)
	return value
}
