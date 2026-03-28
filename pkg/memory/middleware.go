package memory

import (
	"context"
	"fmt"
	"time"

	"github.com/kevinelliott/agentpipe/pkg/agent"
	"github.com/kevinelliott/agentpipe/pkg/log"
	"github.com/kevinelliott/agentpipe/pkg/middleware"
)

// StorageMiddleware stores every agent message in mem0-brain.
// Runs asynchronously to avoid blocking the conversation.
func StorageMiddleware(client *Client) middleware.Middleware {
	return middleware.NewMiddlewareFunc("memory-storage", func(ctx *middleware.MessageContext, msg *agent.Message, next middleware.ProcessFunc) (*agent.Message, error) {
		// Process the message first (don't block on storage)
		result, err := next(ctx, msg)
		if err != nil {
			return result, err
		}

		// Store asynchronously — mem0 can take 90-140s
		if client.IsEnabled() && result != nil && result.Content != "" {
			go func() {
				storeCtx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
				defer cancel()

				storeErr := client.Remember(storeCtx, RememberRequest{
					Content: fmt.Sprintf("[%s] %s: %s", result.AgentType, result.AgentName, result.Content),
					Source:  "agentpipe",
					Date:    time.Now().Format("2006-01-02"),
					Metadata: map[string]string{
						"agent_id":   result.AgentID,
						"agent_name": result.AgentName,
						"agent_type": result.AgentType,
						"turn":       fmt.Sprintf("%d", ctx.TurnNumber),
					},
				})
				if storeErr != nil {
					log.WithFields(map[string]interface{}{
						"agent_id": result.AgentID,
						"error":    storeErr.Error(),
					}).Warn("failed to store memory (non-blocking)")
				}
			}()
		}

		return result, nil
	})
}

// EnrichmentMiddleware injects relevant memories into agent prompts.
// It queries mem0-brain for context relevant to the current conversation
// and prepends it to the message content.
func EnrichmentMiddleware(client *Client, queryFromPrompt func([]agent.Message) string) middleware.Middleware {
	return middleware.NewMiddlewareFunc("memory-enrichment", func(ctx *middleware.MessageContext, msg *agent.Message, next middleware.ProcessFunc) (*agent.Message, error) {
		if !client.IsEnabled() {
			return next(ctx, msg)
		}

		// Only enrich for user/system messages going to agents
		if msg.Role != "user" && msg.Role != "system" {
			return next(ctx, msg)
		}

		// Build query from message content
		query := msg.Content
		if queryFromPrompt != nil {
			// Custom query builder (e.g., extract key topics)
			query = queryFromPrompt([]agent.Message{*msg})
		}

		// Recall relevant memories
		recallCtx, cancel := context.WithTimeout(ctx.Ctx, 10*time.Second)
		defer cancel()

		contextStr, err := client.RecallForContext(recallCtx, query, 5)
		if err != nil {
			log.WithFields(map[string]interface{}{
				"error": err.Error(),
			}).Debug("memory recall failed, continuing without context")
			return next(ctx, msg)
		}

		if contextStr != "" {
			enriched := *msg
			enriched.Content = contextStr + "\n---\n\n" + msg.Content
			return next(ctx, &enriched)
		}

		return next(ctx, msg)
	})
}

// SummaryMiddleware stores conversation summaries in mem0-brain
// when a conversation ends.
func SummaryMiddleware(client *Client) middleware.Middleware {
	return middleware.NewMiddlewareFunc("memory-summary", func(ctx *middleware.MessageContext, msg *agent.Message, next middleware.ProcessFunc) (*agent.Message, error) {
		result, err := next(ctx, msg)
		if err != nil {
			return result, err
		}

		// Detect conversation end signals
		if result != nil && result.Role == "system" && result.Content != "" {
			isSummary := len(result.Content) > 200 // summaries are typically longer
			if isSummary && client.IsEnabled() {
				go func() {
					storeCtx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
					defer cancel()

					client.Remember(storeCtx, RememberRequest{
						Content: fmt.Sprintf("[conversation-summary] %s", result.Content),
						Source:  "agentpipe",
						Date:    time.Now().Format("2006-01-02"),
						Tags:    []string{"summary", "conversation"},
					})
				}()
			}
		}

		return result, nil
	})
}
