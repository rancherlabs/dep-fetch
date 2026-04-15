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

// scalarLocation records where a scalar value sits in the raw file.
type scalarLocation struct {
	line  int        // 1-indexed, matches yaml.Node.Line
	value string     // unquoted value from the AST
	style yaml.Style // original quoting style
}

// toolLocations holds the file positions of a tool's mutable fields.
// This is the only type that depends on the YAML library — swapping to a
// go-yaml AST approach means replacing locateToolFields and this struct.
type toolLocations struct {
	version   scalarLocation
	checksums map[string]scalarLocation // platform key -> checksum value location
}

// locateToolFields parses data and returns the file locations of the named
// tool's version and checksum fields. This is the sole YAML-library-specific
// function; everything that follows is plain string manipulation.
func locateToolFields(data []byte, toolName string) (*toolLocations, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}

	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, fmt.Errorf("invalid YAML document")
	}

	root := doc.Content[0]
	var toolsNode *yaml.Node
	for i := 0; i < len(root.Content)-1; i += 2 {
		if root.Content[i].Value == "tools" {
			toolsNode = root.Content[i+1]
			break
		}
	}
	if toolsNode == nil || toolsNode.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("could not find tools sequence in config")
	}

	for _, toolNode := range toolsNode.Content {
		var nameNode, versionNode, checksumsNode *yaml.Node
		for i := 0; i < len(toolNode.Content)-1; i += 2 {
			switch toolNode.Content[i].Value {
			case "name":
				nameNode = toolNode.Content[i+1]
			case "version":
				versionNode = toolNode.Content[i+1]
			case "checksums":
				checksumsNode = toolNode.Content[i+1]
			}
		}

		if nameNode == nil || nameNode.Value != toolName {
			continue
		}
		if versionNode == nil {
			return nil, fmt.Errorf("tool %q has no version field", toolName)
		}

		locs := &toolLocations{
			version: scalarLocation{
				line:  versionNode.Line,
				value: versionNode.Value,
				style: versionNode.Style,
			},
			checksums: make(map[string]scalarLocation),
		}

		if checksumsNode != nil {
			for i := 0; i < len(checksumsNode.Content)-1; i += 2 {
				plat := checksumsNode.Content[i].Value
				val := checksumsNode.Content[i+1]
				locs.checksums[plat] = scalarLocation{
					line:  val.Line,
					value: val.Value,
					style: val.Style,
				}
			}
		}

		return locs, nil
	}

	return nil, fmt.Errorf("tool %q not found in YAML", toolName)
}

// UpdateToolVersion rewrites the config file in-place, updating the named
// tool's version and checksums while preserving all other formatting and comments.
func UpdateToolVersion(fs billy.Filesystem, cfg *Config, toolName, version string, checksums map[string]string) error {
	f, err := fs.Open(cfg.filePath)
	if err != nil {
		return err
	}
	data, err := io.ReadAll(f)
	f.Close() //nolint:errcheck,gosec // read-only; close error is not actionable
	if err != nil {
		return err
	}

	locs, err := locateToolFields(data, toolName)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	applyEdits(lines, locs, version, checksums)

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

// applyEdits patches lines in-place using the field locations returned by
// locateToolFields. No YAML library dependency — pure string manipulation.
func applyEdits(lines []string, locs *toolLocations, version string, checksums map[string]string) {
	lineIdx := locs.version.line - 1
	old, replacement := quotedPair(locs.version, version)
	lines[lineIdx] = strings.Replace(lines[lineIdx], old, replacement, 1)

	for plat, loc := range locs.checksums {
		sum, ok := checksums[plat]
		if !ok {
			continue
		}
		lineIdx := loc.line - 1
		old, replacement := quotedPair(loc, sum)
		lines[lineIdx] = strings.Replace(lines[lineIdx], old, replacement, 1)
		lines[lineIdx] = renovateLocalRe.ReplaceAllString(lines[lineIdx], "${1}="+version)
	}
}

// quotedPair returns the search string and replacement for a scalar location,
// preserving the original quoting style.
func quotedPair(loc scalarLocation, newVal string) (search, replacement string) {
	switch loc.style {
	case yaml.DoubleQuotedStyle:
		return `"` + loc.value + `"`, `"` + newVal + `"`
	case yaml.SingleQuotedStyle:
		return `'` + loc.value + `'`, `'` + newVal + `'`
	default:
		return loc.value, newVal
	}
}
