package terminal

import (
	"github.com/kevinelliott/agentpipe/pkg/agent"
	"github.com/kevinelliott/agentpipe/pkg/middleware"
)

// ContextInjectionMiddleware prepends recent terminal output to messages
// sent to agents, giving them visibility into the shared terminal state.
func ContextInjectionMiddleware(session *Session, historyLines int) middleware.Middleware {
	if historyLines == 0 {
		historyLines = 30
	}

	return middleware.NewMiddlewareFunc("terminal-context", func(ctx *middleware.MessageContext, msg *agent.Message, next middleware.ProcessFunc) (*agent.Message, error) {
		// Only inject terminal context for user/system messages going to agents
		if msg.Role != "user" && msg.Role != "system" {
			return next(ctx, msg)
		}

		termContext := session.GetContextString(historyLines)
		if termContext == "" {
			return next(ctx, msg)
		}

		enriched := *msg
		enriched.Content = termContext + "\n---\n\n" + msg.Content
		return next(ctx, &enriched)
	})
}
