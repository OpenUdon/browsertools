package browsertools

import (
	"bytes"
	"encoding/json"
	"io"
	"math/big"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/OpenUdon/browsertools/internal/strictjson"
	"github.com/OpenUdon/uws/binding"
	"gopkg.in/yaml.v3"
)

func canonicalBrowserShapeJSON(value any) ([]byte, error) {
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, ErrBrowserShapeTable
	}
	return bytes.TrimSuffix(output.Bytes(), []byte("\n")), nil
}

func decodeBrowserShapeDocument(data []byte) (map[string]any, error) {
	if len(data) == 0 || len(data) > binding.MaxTableBytes || !utf8.Valid(data) {
		return nil, ErrBrowserShapeTable
	}
	trimmed := bytes.TrimSpace(data)
	var value any
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		if strictjson.Validate(data, binding.MaxTableBytes, 64) != nil || !browserShapeUnicodeEscapes(data) {
			return nil, ErrBrowserShapeTable
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		if decoder.Decode(&value) != nil {
			return nil, ErrBrowserShapeTable
		}
	} else {
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		var document, extra yaml.Node
		if decoder.Decode(&document) != nil || decoder.Decode(&extra) != io.EOF || len(document.Content) != 1 {
			return nil, ErrBrowserShapeTable
		}
		var err error
		value, err = browserShapeYAML(document.Content[0], 0)
		if err != nil {
			return nil, err
		}
	}
	root, ok := value.(map[string]any)
	if !ok {
		return nil, ErrBrowserShapeTable
	}
	return root, nil
}

// encoding/json substitutes malformed UTF16 escapes. Refuse them before
// decoding, so invalid bytes cannot acquire a valid replacement-character hash.
// JSON syntax and duplicate keys are checked separately by strictjson.Validate.
func browserShapeUnicodeEscapes(data []byte) bool {
	quoted := false
	for i := 0; i < len(data); i++ {
		if data[i] == '"' {
			quoted = !quoted
		} else if quoted && data[i] == '\\' {
			i++
			if i >= len(data) {
				return false
			}
			if data[i] != 'u' {
				continue
			}
			if i+4 >= len(data) {
				return false
			}
			n, err := strconv.ParseUint(string(data[i+1:i+5]), 16, 16)
			if err != nil {
				return false
			}
			i += 4
			if n >= 0xdc00 && n <= 0xdfff {
				return false
			}
			if n >= 0xd800 && n <= 0xdbff {
				if i+6 >= len(data) || data[i+1] != '\\' || data[i+2] != 'u' {
					return false
				}
				low, err := strconv.ParseUint(string(data[i+3:i+7]), 16, 16)
				if err != nil || !utf16.IsSurrogate(rune(low)) || low < 0xdc00 {
					return false
				}
				i += 6
			}
		}
	}
	return true
}

func browserShapeYAML(node *yaml.Node, depth int) (any, error) {
	if depth >= 64 || node.Anchor != "" {
		return nil, ErrBrowserShapeTable
	}
	switch node.Kind {
	case yaml.MappingNode:
		if node.Tag != "!!map" || len(node.Content)%2 != 0 {
			return nil, ErrBrowserShapeTable
		}
		value := map[string]any{}
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || key.Anchor != "" {
				return nil, ErrBrowserShapeTable
			}
			if _, duplicate := value[key.Value]; duplicate {
				return nil, ErrBrowserShapeTable
			}
			child, err := browserShapeYAML(node.Content[i+1], depth+1)
			if err != nil {
				return nil, err
			}
			value[key.Value] = child
		}
		return value, nil
	case yaml.SequenceNode:
		if node.Tag != "!!seq" {
			return nil, ErrBrowserShapeTable
		}
		value := make([]any, 0, len(node.Content))
		for _, child := range node.Content {
			item, err := browserShapeYAML(child, depth+1)
			if err != nil {
				return nil, err
			}
			value = append(value, item)
		}
		return value, nil
	case yaml.ScalarNode:
		switch node.Tag {
		case "!!str", "!!timestamp":
			return node.Value, nil
		case "!!null":
			return nil, nil
		case "!!bool":
			if strings.EqualFold(node.Value, "true") || strings.EqualFold(node.Value, "false") {
				return strings.EqualFold(node.Value, "true"), nil
			}
		case "!!int", "!!float":
			// JSON-shaped YAML numeric tokens retain their lexemes. Other finite
			// YAML integers normalize exactly, with no binary float conversion.
			if json.Valid([]byte(node.Value)) {
				return json.Number(node.Value), nil
			}
			if node.Tag == "!!int" {
				n, ok := new(big.Int).SetString(strings.ReplaceAll(node.Value, "_", ""), 0)
				if ok {
					return json.Number(n.String()), nil
				}
			}
		}
	}
	return nil, ErrBrowserShapeTable
}
