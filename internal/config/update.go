package config

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/go-git/go-billy/v5"
	"go.yaml.in/yaml/v3"
)

var renovateLocalRe = regexp.MustCompile(`(renovate-local:\s*[a-zA-Z0-9_-]+)=.*`)

// UpdateToolVersion rewrites the config file in-place, updating the named tool's version
// and checksums while preserving all other formatting and comments.
func UpdateToolVersion(fs billy.Filesystem, cfg *Config, toolName, version string, checksums map[string]string) error {
	f, err := fs.Open(cfg.filePath)
	if err != nil {
		return err
	}
	data, err := io.ReadAll(f)
	f.Close() //nolint:errcheck // read-only; close error is not actionable
	if err != nil {
		return err
	}

	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return err
	}

	if node.Kind != yaml.DocumentNode || len(node.Content) == 0 {
		return fmt.Errorf("invalid YAML document")
	}

	root := node.Content[0]
	var toolsNode *yaml.Node
	for i := 0; i < len(root.Content); i += 2 {
		if root.Content[i].Value == "tools" {
			toolsNode = root.Content[i+1]
			break
		}
	}

	if toolsNode == nil || toolsNode.Kind != yaml.SequenceNode {
		return fmt.Errorf("could not find tools sequence in config")
	}

	var foundToolNode *yaml.Node
	for _, toolNode := range toolsNode.Content {
		for i := 0; i < len(toolNode.Content); i += 2 {
			if toolNode.Content[i].Value == "name" && toolNode.Content[i+1].Value == toolName {
				foundToolNode = toolNode
				break
			}
		}
		if foundToolNode != nil {
			break
		}
	}

	if foundToolNode == nil {
		return fmt.Errorf("tool %q not found in YAML", toolName)
	}

	// Use line indexes from the parsed AST to replace values as strings, preserving
	// all surrounding formatting and comments.
	lines := strings.Split(string(data), "\n")

	// Update version field.
	for i := 0; i < len(foundToolNode.Content); i += 2 {
		if foundToolNode.Content[i].Value == "version" {
			valNode := foundToolNode.Content[i+1]
			lineIdx := valNode.Line - 1
			searchVal, newValStr := quotedPair(valNode, version)
			lines[lineIdx] = strings.Replace(lines[lineIdx], searchVal, newValStr, 1)
			break
		}
	}

	// Update checksum values, and any inline renovate-local pin comments.
	if len(checksums) > 0 {
		for i := 0; i < len(foundToolNode.Content); i += 2 {
			if foundToolNode.Content[i].Value == "checksums" {
				checksumsNode := foundToolNode.Content[i+1]
				for j := 0; j < len(checksumsNode.Content); j += 2 {
					plat := checksumsNode.Content[j].Value
					valNode := checksumsNode.Content[j+1]
					if sum, ok := checksums[plat]; ok {
						lineIdx := valNode.Line - 1
						searchVal, newValStr := quotedPair(valNode, sum)
						lines[lineIdx] = strings.Replace(lines[lineIdx], searchVal, newValStr, 1)
						lines[lineIdx] = renovateLocalRe.ReplaceAllString(lines[lineIdx], "${1}="+version)
					}
				}
				break
			}
		}
	}

	wf, err := fs.OpenFile(cfg.filePath, os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	_, werr := wf.Write([]byte(strings.Join(lines, "\n")))
	cerr := wf.Close()
	if werr != nil {
		return werr
	}
	return cerr
}

// quotedPair returns the search string and replacement string for a YAML scalar node,
// preserving the original quoting style.
func quotedPair(node *yaml.Node, newVal string) (search, replacement string) {
	old := node.Value
	switch node.Style {
	case yaml.DoubleQuotedStyle:
		return `"` + old + `"`, `"` + newVal + `"`
	case yaml.SingleQuotedStyle:
		return `'` + old + `'`, `'` + newVal + `'`
	default:
		return old, newVal
	}
}
