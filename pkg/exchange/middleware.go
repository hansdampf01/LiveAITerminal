package exchange

import (
	"github.com/kevinelliott/agentpipe/pkg/agent"
	"github.com/kevinelliott/agentpipe/pkg/middleware"
)

// ExchangeMiddleware injects EXCHANGE.md content into agent prompts
// so each agent knows what the others have been working on.
func ExchangeMiddleware(manager *Manager) middleware.Middleware {
	return middleware.NewMiddlewareFunc("exchange", func(ctx *middleware.MessageContext, msg *agent.Message, next middleware.ProcessFunc) (*agent.Message, error) {
		// Only inject for messages going to agents
		if msg.Role != "user" && msg.Role != "system" {
			return next(ctx, msg)
		}

		contextStr := manager.GetContextString()
		if contextStr == "" {
			return next(ctx, msg)
		}

		enriched := *msg
		enriched.Content = contextStr + "\n---\n\n" + msg.Content
		return next(ctx, &enriched)
	})
}
