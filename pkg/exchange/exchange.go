// Package exchange manages an EXCHANGE.md file for structured coordination
// between AI agents working on the same project.
package exchange

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Manager handles reading and writing the EXCHANGE.md coordination file.
type Manager struct {
	filePath string
	mu       sync.Mutex
}

// NewManager creates a new exchange manager.
func NewManager(projectDir, fileName string) *Manager {
	if fileName == "" {
		fileName = "EXCHANGE.md"
	}
	return &Manager{
		filePath: filepath.Join(projectDir, fileName),
	}
}

// Read returns the current content of EXCHANGE.md, or empty string if not found.
func (m *Manager) Read() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.filePath)
	if err != nil {
		return ""
	}
	return string(data)
}

// ReadSection returns the content of a specific agent's section.
func (m *Manager) ReadSection(agentName string) string {
	content := m.Read()
	if content == "" {
		return ""
	}

	header := fmt.Sprintf("## %s", agentName)
	idx := strings.Index(content, header)
	if idx == -1 {
		return ""
	}

	// Find the end of this section (next ## header or end of file)
	rest := content[idx+len(header):]
	nextHeader := strings.Index(rest, "\n## ")
	if nextHeader == -1 {
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(rest[:nextHeader])
}

// UpdateSection writes or overwrites an agent's section in EXCHANGE.md.
func (m *Manager) UpdateSection(agentName, status, completedWork, nextSteps string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	content, _ := os.ReadFile(m.filePath)
	existing := string(content)

	// Build the new section
	var section strings.Builder
	fmt.Fprintf(&section, "## %s\n", agentName)
	fmt.Fprintf(&section, "**Updated**: %s\n\n", time.Now().Format("15:04:05"))
	if status != "" {
		fmt.Fprintf(&section, "**Status**: %s\n\n", status)
	}
	if completedWork != "" {
		fmt.Fprintf(&section, "### Completed\n%s\n\n", completedWork)
	}
	if nextSteps != "" {
		fmt.Fprintf(&section, "### Next Steps\n%s\n\n", nextSteps)
	}

	header := fmt.Sprintf("## %s", agentName)
	idx := strings.Index(existing, header)

	if idx == -1 {
		// Append new section
		if existing != "" && !strings.HasSuffix(existing, "\n") {
			existing += "\n"
		}
		existing += "\n" + section.String()
	} else {
		// Replace existing section
		rest := existing[idx+len(header):]
		nextHeader := strings.Index(rest, "\n## ")

		var result strings.Builder
		result.WriteString(existing[:idx])
		result.WriteString(section.String())
		if nextHeader != -1 {
			result.WriteString(rest[nextHeader:])
		}
		existing = result.String()
	}

	// Ensure header exists
	if !strings.HasPrefix(existing, "# ") {
		existing = "# EXCHANGE — Agent Coordination\n\n" + existing
	}

	return os.WriteFile(m.filePath, []byte(existing), 0644)
}

// GetContextString returns the full EXCHANGE.md content formatted for prompt injection.
func (m *Manager) GetContextString() string {
	content := m.Read()
	if content == "" {
		return ""
	}

	return fmt.Sprintf("## Current EXCHANGE.md (Coordination File)\n\n%s\n", content)
}
