// Package context provides cross-AI context bridging.
// It tracks what each agent knows, shares decision rationale between agents,
// and builds agent profiles from conversation history.
package context

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/kevinelliott/agentpipe/pkg/agent"
	"github.com/kevinelliott/agentpipe/pkg/middleware"
)

// AgentProfile tracks an agent's contributions and expertise.
type AgentProfile struct {
	AgentID       string    `json:"agent_id"`
	AgentName     string    `json:"agent_name"`
	AgentType     string    `json:"agent_type"`
	Contributions int       `json:"contributions"`
	LastActive    time.Time `json:"last_active"`
	Topics        []string  `json:"topics"`    // Key topics this agent has discussed
	Decisions     []string  `json:"decisions"` // Decisions this agent has made
}

// SharedContext holds information shared across all agents.
type SharedContext struct {
	ProjectGoal string            `json:"project_goal,omitempty"`
	KeyFacts    map[string]string `json:"key_facts"` // key -> value
	OpenIssues  []string          `json:"open_issues"`
}

// Bridge manages cross-agent context sharing.
// It tracks agent profiles, shared facts, and enables agents to build
// on each other's work.
type Bridge struct {
	profiles      map[string]*AgentProfile
	shared        SharedContext
	recentOutputs map[string][]string // agentID -> recent output lines
	maxRecent     int
	mu            sync.RWMutex
}

// NewBridge creates a new context bridge.
func NewBridge() *Bridge {
	return &Bridge{
		profiles: make(map[string]*AgentProfile),
		shared: SharedContext{
			KeyFacts: make(map[string]string),
		},
		recentOutputs: make(map[string][]string),
		maxRecent:     20,
	}
}

// TrackAgent updates an agent's profile based on a message.
func (b *Bridge) TrackAgent(msg *agent.Message) {
	if msg == nil || msg.AgentID == "" {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	profile, exists := b.profiles[msg.AgentID]
	if !exists {
		profile = &AgentProfile{
			AgentID:   msg.AgentID,
			AgentName: msg.AgentName,
			AgentType: msg.AgentType,
		}
		b.profiles[msg.AgentID] = profile
	}

	profile.Contributions++
	profile.LastActive = time.Now()

	// Track recent outputs per agent
	outputs := b.recentOutputs[msg.AgentID]
	outputs = append(outputs, msg.Content)
	if len(outputs) > b.maxRecent {
		outputs = outputs[len(outputs)-b.maxRecent:]
	}
	b.recentOutputs[msg.AgentID] = outputs
}

// SetProjectGoal sets the shared project goal.
func (b *Bridge) SetProjectGoal(goal string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.shared.ProjectGoal = goal
}

// SetFact stores a shared fact.
func (b *Bridge) SetFact(key, value string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.shared.KeyFacts[key] = value
}

// AddIssue adds an open issue to shared context.
func (b *Bridge) AddIssue(issue string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.shared.OpenIssues = append(b.shared.OpenIssues, issue)
}

// ResolveIssue removes an issue from the open list.
func (b *Bridge) ResolveIssue(index int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if index >= 0 && index < len(b.shared.OpenIssues) {
		b.shared.OpenIssues = append(b.shared.OpenIssues[:index], b.shared.OpenIssues[index+1:]...)
	}
}

// GetContextForAgent returns a formatted context string for a specific agent.
// It includes other agents' recent outputs and shared context,
// but excludes the agent's own output (it already knows that).
func (b *Bridge) GetContextForAgent(agentID string) string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var buf strings.Builder

	// Shared context
	if b.shared.ProjectGoal != "" {
		fmt.Fprintf(&buf, "## Project Goal\n%s\n\n", b.shared.ProjectGoal)
	}

	if len(b.shared.KeyFacts) > 0 {
		buf.WriteString("## Key Facts\n")
		for k, v := range b.shared.KeyFacts {
			fmt.Fprintf(&buf, "- **%s**: %s\n", k, v)
		}
		buf.WriteString("\n")
	}

	if len(b.shared.OpenIssues) > 0 {
		buf.WriteString("## Open Issues\n")
		for i, issue := range b.shared.OpenIssues {
			fmt.Fprintf(&buf, "%d. %s\n", i+1, issue)
		}
		buf.WriteString("\n")
	}

	// Other agents' recent work
	hasOtherAgents := false
	for id, profile := range b.profiles {
		if id == agentID {
			continue
		}
		outputs := b.recentOutputs[id]
		if len(outputs) == 0 {
			continue
		}

		if !hasOtherAgents {
			buf.WriteString("## Other Agents' Recent Work\n\n")
			hasOtherAgents = true
		}

		fmt.Fprintf(&buf, "### %s (%s) — %d contributions\n",
			profile.AgentName, profile.AgentType, profile.Contributions)

		// Show last few outputs
		start := len(outputs) - 3
		if start < 0 {
			start = 0
		}
		for _, out := range outputs[start:] {
			// Truncate long outputs
			if len(out) > 200 {
				out = out[:200] + "..."
			}
			fmt.Fprintf(&buf, "> %s\n", out)
		}
		buf.WriteString("\n")
	}

	return buf.String()
}

// GetProfiles returns all agent profiles.
func (b *Bridge) GetProfiles() map[string]*AgentProfile {
	b.mu.RLock()
	defer b.mu.RUnlock()

	out := make(map[string]*AgentProfile, len(b.profiles))
	for k, v := range b.profiles {
		copy := *v
		out[k] = &copy
	}
	return out
}

// ContextBridgeMiddleware injects cross-agent context into messages
// and tracks agent contributions.
func ContextBridgeMiddleware(bridge *Bridge) middleware.Middleware {
	return middleware.NewMiddlewareFunc("context-bridge", func(ctx *middleware.MessageContext, msg *agent.Message, next middleware.ProcessFunc) (*agent.Message, error) {
		// Track this agent's output
		if msg.Role == "agent" {
			bridge.TrackAgent(msg)
		}

		// Inject context for messages going to agents
		if msg.Role == "user" || msg.Role == "system" {
			contextStr := bridge.GetContextForAgent(ctx.AgentID)
			if contextStr != "" {
				enriched := *msg
				enriched.Content = contextStr + "\n---\n\n" + msg.Content
				return next(ctx, &enriched)
			}
		}

		return next(ctx, msg)
	})
}
