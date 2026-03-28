package filewatch

import (
	"github.com/kevinelliott/agentpipe/pkg/agent"
	"github.com/kevinelliott/agentpipe/pkg/log"
	"github.com/kevinelliott/agentpipe/pkg/middleware"
)

// FileChangeMiddleware injects file change information into agent prompts.
// Before each agent turn, it takes a snapshot and reports what changed.
func FileChangeMiddleware(watcher *Watcher) middleware.Middleware {
	return middleware.NewMiddlewareFunc("filewatch", func(ctx *middleware.MessageContext, msg *agent.Message, next middleware.ProcessFunc) (*agent.Message, error) {
		// Only inject for messages going to agents (user/system role)
		if msg.Role != "user" && msg.Role != "system" {
			// For agent responses, take a snapshot after they finish
			result, err := next(ctx, msg)
			if err == nil && msg.Role == "agent" {
				watcher.TakeSnapshot() // capture what the agent changed
			}
			return result, err
		}

		// Take snapshot to see what changed since last turn
		changes, err := watcher.TakeSnapshot()
		if err != nil {
			log.WithFields(map[string]interface{}{
				"error": err.Error(),
			}).Debug("filewatch snapshot failed, continuing without changes")
			return next(ctx, msg)
		}

		if changes != nil && !changes.IsEmpty() {
			enriched := *msg
			enriched.Content = changes.String() + "\n---\n\n" + msg.Content

			log.WithFields(map[string]interface{}{
				"created":  len(changes.Created),
				"modified": len(changes.Modified),
				"deleted":  len(changes.Deleted),
			}).Debug("file changes detected, injecting into prompt")

			return next(ctx, &enriched)
		}

		return next(ctx, msg)
	})
}
