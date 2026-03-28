// Package terminal provides a shared terminal session that multiple AI agents
// can observe and execute commands in. This enables collaborative debugging
// and shared context between agents.
package terminal

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/kevinelliott/agentpipe/pkg/log"
)

// OutputLine represents a single line of terminal output.
type OutputLine struct {
	Content   string    `json:"content"`
	IsError   bool      `json:"is_error"`
	Timestamp time.Time `json:"timestamp"`
	Source    string    `json:"source"` // "user", agent name, or "system"
}

// CommandResult captures the result of a shell command execution.
type CommandResult struct {
	Command  string       `json:"command"`
	Output   []OutputLine `json:"output"`
	ExitCode int          `json:"exit_code"`
	Duration time.Duration `json:"duration"`
	Source   string       `json:"source"`
}

// SessionConfig configures the shared terminal session.
type SessionConfig struct {
	Shell      string `yaml:"shell"`
	WorkingDir string `yaml:"working_dir"`
	MaxHistory int    `yaml:"max_history"` // max lines of output to retain
}

// Observer is notified when new output arrives.
type Observer func(line OutputLine)

// Session is a shared terminal that agents can observe and execute commands in.
type Session struct {
	config    SessionConfig
	history   []OutputLine
	results   []CommandResult
	observers []Observer
	mu        sync.RWMutex
	cwd       string
}

// NewSession creates a new shared terminal session.
func NewSession(config SessionConfig) *Session {
	if config.Shell == "" {
		config.Shell = os.Getenv("SHELL")
		if config.Shell == "" {
			config.Shell = "/bin/sh"
		}
	}
	if config.WorkingDir == "" {
		config.WorkingDir, _ = os.Getwd()
	}
	if config.MaxHistory == 0 {
		config.MaxHistory = 500
	}

	return &Session{
		config:  config,
		history: make([]OutputLine, 0, config.MaxHistory),
		results: make([]CommandResult, 0),
		cwd:     config.WorkingDir,
	}
}

// AddObserver registers a callback for new output lines.
func (s *Session) AddObserver(obs Observer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.observers = append(s.observers, obs)
}

// Execute runs a command in the shared shell and captures output.
func (s *Session) Execute(ctx context.Context, command string, source string) (*CommandResult, error) {
	start := time.Now()

	cmd := exec.CommandContext(ctx, s.config.Shell, "-c", command)
	cmd.Dir = s.cwd
	cmd.Env = append(os.Environ(), "TERM=dumb")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start command: %w", err)
	}

	result := &CommandResult{
		Command: command,
		Source:  source,
	}

	// Read stdout and stderr concurrently
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := OutputLine{
				Content:   scanner.Text(),
				IsError:   false,
				Timestamp: time.Now(),
				Source:    source,
			}
			s.addLine(line)
			result.Output = append(result.Output, line)
		}
	}()

	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := OutputLine{
				Content:   scanner.Text(),
				IsError:   true,
				Timestamp: time.Now(),
				Source:    source,
			}
			s.addLine(line)
			result.Output = append(result.Output, line)
		}
	}()

	wg.Wait()
	err = cmd.Wait()

	result.ExitCode = cmd.ProcessState.ExitCode()
	result.Duration = time.Since(start)

	s.mu.Lock()
	s.results = append(s.results, *result)
	s.mu.Unlock()

	log.WithFields(map[string]interface{}{
		"command":   command,
		"source":    source,
		"exit_code": result.ExitCode,
		"duration":  result.Duration.String(),
	}).Debug("command executed")

	return result, err
}

// GetHistory returns recent terminal output.
func (s *Session) GetHistory(lines int) []OutputLine {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if lines <= 0 || lines > len(s.history) {
		lines = len(s.history)
	}
	start := len(s.history) - lines
	out := make([]OutputLine, lines)
	copy(out, s.history[start:])
	return out
}

// GetContextString returns recent terminal output as a formatted string
// suitable for injecting into agent prompts.
func (s *Session) GetContextString(lines int) string {
	history := s.GetHistory(lines)
	if len(history) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Recent Terminal Output\n\n```\n")
	for _, line := range history {
		prefix := ""
		if line.IsError {
			prefix = "[stderr] "
		}
		fmt.Fprintf(&b, "%s%s\n", prefix, line.Content)
	}
	b.WriteString("```\n")
	fmt.Fprintf(&b, "\nWorking directory: %s\n", s.cwd)

	return b.String()
}

// GetLastResult returns the most recent command result.
func (s *Session) GetLastResult() *CommandResult {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.results) == 0 {
		return nil
	}
	result := s.results[len(s.results)-1]
	return &result
}

// GetWorkingDir returns the current working directory.
func (s *Session) GetWorkingDir() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cwd
}

// SetWorkingDir changes the working directory for future commands.
func (s *Session) SetWorkingDir(dir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cwd = dir
}

func (s *Session) addLine(line OutputLine) {
	s.mu.Lock()
	s.history = append(s.history, line)
	if len(s.history) > s.config.MaxHistory {
		s.history = s.history[len(s.history)-s.config.MaxHistory:]
	}
	observers := make([]Observer, len(s.observers))
	copy(observers, s.observers)
	s.mu.Unlock()

	// Notify observers outside the lock
	for _, obs := range observers {
		obs(line)
	}
}
