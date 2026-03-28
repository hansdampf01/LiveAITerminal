// Package memory provides integration with mem0-brain for persistent AI memory.
// It allows agents to store and recall knowledge across conversations.
package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/kevinelliott/agentpipe/pkg/log"
)

// Memory represents a single memory entry from mem0-brain.
type Memory struct {
	ID        string                 `json:"id"`
	Content   string                 `json:"content"`
	Source    string                 `json:"source,omitempty"`
	Date      string                 `json:"date,omitempty"`
	Tags      []string               `json:"tags,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt string                 `json:"created_at,omitempty"`
}

// RememberRequest is the payload for storing a memory.
type RememberRequest struct {
	Content  string            `json:"content"`
	Source   string            `json:"source,omitempty"`
	Date     string            `json:"date,omitempty"`
	Tags     []string          `json:"tags,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// RecallRequest is the payload for searching memories.
type RecallRequest struct {
	Query  string `json:"query"`
	Limit  int    `json:"limit,omitempty"`
	Source string `json:"source,omitempty"`
}

// RecallResponse is the response from a recall request.
type RecallResponse struct {
	Memories []Memory `json:"memories"`
}

// HealthResponse is the response from the health endpoint.
type HealthResponse struct {
	Status string `json:"status"`
}

// ClientConfig configures the mem0-brain client.
type ClientConfig struct {
	// BaseURL is the mem0-brain API base URL (e.g., "https://brain.jajoe.xyz/api/v1")
	BaseURL string `yaml:"base_url"`
	// Token is the Bearer token for authentication
	Token string `yaml:"token"`
	// Timeout for HTTP requests (default: 180s because mem0 can be slow)
	Timeout time.Duration `yaml:"timeout"`
	// Source identifies this client (e.g., "agentpipe")
	Source string `yaml:"source"`
	// Enabled toggles memory integration
	Enabled bool `yaml:"enabled"`
}

// Client communicates with the mem0-brain API.
type Client struct {
	config     ClientConfig
	httpClient *http.Client
}

// NewClient creates a new mem0-brain client.
func NewClient(config ClientConfig) *Client {
	timeout := config.Timeout
	if timeout == 0 {
		timeout = 180 * time.Second // mem0 can take 90-140s
	}
	if config.Source == "" {
		config.Source = "agentpipe"
	}

	return &Client{
		config: config,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// IsEnabled returns whether memory integration is active.
func (c *Client) IsEnabled() bool {
	return c.config.Enabled && c.config.BaseURL != "" && c.config.Token != ""
}

// Health checks if mem0-brain is reachable.
func (c *Client) Health(ctx context.Context) error {
	resp, err := c.doRequest(ctx, "GET", "/health", nil)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}
	return nil
}

// Remember stores a memory in mem0-brain.
func (c *Client) Remember(ctx context.Context, req RememberRequest) error {
	if req.Source == "" {
		req.Source = c.config.Source
	}
	if req.Date == "" {
		req.Date = time.Now().Format("2006-01-02")
	}

	resp, err := c.doRequest(ctx, "POST", "/memory/remember", req)
	if err != nil {
		return fmt.Errorf("remember failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("remember returned status %d: %s", resp.StatusCode, string(body))
	}

	log.WithFields(map[string]interface{}{
		"source": req.Source,
	}).Debug("memory stored successfully")

	return nil
}

// Recall searches memories in mem0-brain.
func (c *Client) Recall(ctx context.Context, req RecallRequest) ([]Memory, error) {
	if req.Limit == 0 {
		req.Limit = 10
	}

	resp, err := c.doRequest(ctx, "POST", "/memory/recall", req)
	if err != nil {
		return nil, fmt.Errorf("recall failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("recall returned status %d: %s", resp.StatusCode, string(body))
	}

	var result RecallResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode recall response: %w", err)
	}

	return result.Memories, nil
}

// RecallForContext searches memories and formats them as context string
// suitable for injecting into agent prompts.
func (c *Client) RecallForContext(ctx context.Context, query string, limit int) (string, error) {
	memories, err := c.Recall(ctx, RecallRequest{
		Query: query,
		Limit: limit,
	})
	if err != nil {
		return "", err
	}

	if len(memories) == 0 {
		return "", nil
	}

	var buf bytes.Buffer
	buf.WriteString("## Relevant Memories\n\n")
	for i, m := range memories {
		fmt.Fprintf(&buf, "%d. %s", i+1, m.Content)
		if m.Source != "" {
			fmt.Fprintf(&buf, " (source: %s)", m.Source)
		}
		if m.Date != "" {
			fmt.Fprintf(&buf, " [%s]", m.Date)
		}
		buf.WriteString("\n")
	}

	return buf.String(), nil
}

func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	url := c.config.BaseURL + path

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.config.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.httpClient.Do(req)
}
