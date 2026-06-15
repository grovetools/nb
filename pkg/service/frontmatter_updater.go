package service

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// ValidPriorities is the ordered set of accepted note priority values
// (p0 = most critical). An empty string clears the priority.
var ValidPriorities = []string{"p0", "p1", "p2", "p3"}

// IsValidPriority reports whether p is an accepted priority value. The empty
// string is valid (it clears the field).
func IsValidPriority(p string) bool {
	if p == "" {
		return true
	}
	for _, v := range ValidPriorities {
		if v == p {
			return true
		}
	}
	return false
}

// UpdateNotePriority sets (or clears, when priority == "") the `priority`
// frontmatter field on the note at path, preserving all other frontmatter via
// the formatting-preserving updateFrontmatterFields helper.
func (s *Service) UpdateNotePriority(path, priority string) error {
	if !IsValidPriority(priority) {
		return fmt.Errorf("invalid priority %q (want one of p0,p1,p2,p3 or empty)", priority)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read note for priority update: %w", err)
	}

	newContent, err := updateFrontmatterFields(content, map[string]interface{}{"priority": priority})
	if err != nil {
		return fmt.Errorf("update priority frontmatter: %w", err)
	}

	if err := os.WriteFile(path, newContent, 0o644); err != nil {
		return fmt.Errorf("write note with updated priority: %w", err)
	}
	return nil
}

// parseFrontmatterToMap extracts YAML frontmatter from markdown content.
// Returns the parsed YAML as a map, the remaining content, and any error.
func parseFrontmatterToMap(content []byte) (map[string]interface{}, []byte, error) {
	contentStr := string(content)

	// Check if the file starts with frontmatter delimiter
	if !strings.HasPrefix(contentStr, "---\n") && !strings.HasPrefix(contentStr, "---\r\n") {
		// No frontmatter, return empty map and full content
		return make(map[string]interface{}), content, nil
	}

	// Find the closing delimiter
	startIdx := strings.Index(contentStr, "\n") + 1
	if startIdx == 0 {
		return nil, nil, fmt.Errorf("invalid frontmatter: no newline after opening delimiter")
	}

	// Look for the closing "---" on its own line
	var endIdx int
	if strings.HasPrefix(contentStr[startIdx:], "---\n") {
		endIdx = startIdx
	} else {
		tmpIdx := strings.Index(contentStr[startIdx:], "\n---\n")
		if tmpIdx == -1 {
			tmpIdx = strings.Index(contentStr[startIdx:], "\r\n---\r\n")
			if tmpIdx == -1 {
				return nil, nil, fmt.Errorf("invalid frontmatter: no closing delimiter found")
			}
		}
		endIdx = startIdx + tmpIdx
	}

	// Extract the YAML content
	yamlContent := contentStr[startIdx:endIdx]

	// Parse the YAML
	var frontmatter map[string]interface{}
	if yamlContent == "" {
		frontmatter = make(map[string]interface{})
	} else if err := yaml.Unmarshal([]byte(yamlContent), &frontmatter); err != nil {
		return nil, nil, fmt.Errorf("parsing frontmatter: %w", err)
	}

	// Find where the body content starts
	var bodyStart int
	if endIdx == startIdx {
		bodyStart = startIdx + 4 // Skip "---\n"
	} else {
		bodyStart = endIdx + 5 // length of "\n---\n"
	}
	if bodyStart > len(contentStr) {
		bodyStart = len(contentStr)
	}

	remainingContent := []byte(contentStr[bodyStart:])

	return frontmatter, remainingContent, nil
}

// updateFrontmatterFields updates specific fields in the frontmatter while preserving existing ones.
func updateFrontmatterFields(content []byte, updates map[string]interface{}) ([]byte, error) {
	// Extract raw frontmatter string
	frontmatterStr, body, err := extractFrontmatterString(content)
	if err != nil {
		return nil, err
	}

	// If no frontmatter exists, create new one
	if frontmatterStr == "" {
		newFrontmatter := make(map[string]interface{})
		for k, v := range updates {
			newFrontmatter[k] = v
		}

		yamlBytes, err := yaml.Marshal(newFrontmatter)
		if err != nil {
			return nil, fmt.Errorf("marshaling new frontmatter: %w", err)
		}

		var result bytes.Buffer
		result.WriteString("---\n")
		result.Write(yamlBytes)
		result.WriteString("---\n")
		result.Write(body)

		return result.Bytes(), nil
	}

	// Update existing frontmatter using Node API for formatting preservation
	updatedYAML, err := updateFrontmatterNode([]byte(frontmatterStr), updates)
	if err != nil {
		return nil, err
	}

	// Reconstruct the file
	return replaceFrontmatter(content, string(updatedYAML)), nil
}

// updateFrontmatterNode updates YAML using the Node API to preserve formatting.
func updateFrontmatterNode(yamlData []byte, updates map[string]interface{}) ([]byte, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(yamlData, &root); err != nil {
		return nil, fmt.Errorf("unmarshaling YAML: %w", err)
	}

	// Navigate to the document content
	if len(root.Content) == 0 {
		return nil, fmt.Errorf("no YAML document found")
	}
	doc := root.Content[0]

	// Update fields in the document
	for key, value := range updates {
		updateNodeValue(doc, key, value)
	}

	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(&root); err != nil {
		return nil, fmt.Errorf("encoding YAML: %w", err)
	}

	return buf.Bytes(), nil
}

// updateNodeValue updates a specific field in a YAML node.
func updateNodeValue(node *yaml.Node, key string, value interface{}) {
	if node.Kind != yaml.MappingNode {
		return
	}

	// Iterate through key-value pairs
	for i := 0; i < len(node.Content)-1; i += 2 {
		keyNode := node.Content[i]
		if keyNode.Value == key {
			// Update the value node
			valueNode := node.Content[i+1]
			valueNode.Kind = yaml.ScalarNode
			valueNode.Value = fmt.Sprint(value)
			valueNode.Tag = resolveYAMLTag(value)
			return
		}
	}

	// Key not found, add it
	keyNode := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: key,
		Tag:   "!!str",
	}

	valueNode := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: fmt.Sprint(value),
		Tag:   resolveYAMLTag(value),
	}

	node.Content = append(node.Content, keyNode, valueNode)
}

// resolveYAMLTag determines the appropriate YAML tag for a value.
func resolveYAMLTag(value interface{}) string {
	switch value.(type) {
	case string:
		return "!!str"
	case int, int64, int32:
		return "!!int"
	case float64, float32:
		return "!!float"
	case bool:
		return "!!bool"
	default:
		return "!!str"
	}
}

// extractFrontmatterString extracts the raw YAML string between delimiters.
func extractFrontmatterString(content []byte) (string, []byte, error) {
	contentStr := string(content)

	if !strings.HasPrefix(contentStr, "---\n") && !strings.HasPrefix(contentStr, "---\r\n") {
		return "", content, nil
	}

	startIdx := strings.Index(contentStr, "\n") + 1
	if startIdx == 0 {
		return "", nil, fmt.Errorf("invalid frontmatter: no newline after opening delimiter")
	}

	endIdx := strings.Index(contentStr[startIdx:], "\n---\n")
	if endIdx == -1 {
		endIdx = strings.Index(contentStr[startIdx:], "\r\n---\r\n")
		if endIdx == -1 {
			return "", nil, fmt.Errorf("invalid frontmatter: no closing delimiter found")
		}
	}
	endIdx += startIdx

	yamlContent := contentStr[startIdx:endIdx]

	bodyStart := endIdx + 5 // length of "\n---\n"
	if bodyStart > len(contentStr) {
		bodyStart = len(contentStr)
	}

	remainingContent := []byte(contentStr[bodyStart:])

	return yamlContent, remainingContent, nil
}

// replaceFrontmatter replaces existing frontmatter with new YAML string.
func replaceFrontmatter(content []byte, newFrontmatter string) []byte {
	_, body, _ := extractFrontmatterString(content)

	var result bytes.Buffer
	result.WriteString("---\n")
	result.WriteString(strings.TrimSpace(newFrontmatter))
	result.WriteString("\n---\n")
	result.Write(body)

	return result.Bytes()
}
