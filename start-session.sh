#!/bin/bash
# LiveAITerminal — Start collaborative AI session
# Usage: ./start-session.sh /path/to/project

PROJECT_DIR="${1:-$(pwd)}"
SESSION="ai-collab"

if [ ! -d "$PROJECT_DIR" ]; then
    echo "Error: $PROJECT_DIR does not exist"
    exit 1
fi

# Kill existing session if any
tmux kill-session -t "$SESSION" 2>/dev/null

# Create session with 3 panes:
#   Top-left:  Claude Code
#   Top-right: Codex CLI
#   Bottom:    LiveAITerminal (monitoring)

tmux new-session -d -s "$SESSION" -c "$PROJECT_DIR" -x 200 -y 50

# Pane 0: Claude Code
tmux send-keys -t "$SESSION" "cd $PROJECT_DIR && claude" Enter

# Split horizontally — Pane 1: Codex CLI
tmux split-window -h -t "$SESSION" -c "$PROJECT_DIR"
tmux send-keys -t "$SESSION" "cd $PROJECT_DIR && codex" Enter

# Split bottom — Pane 2: Monitoring
tmux split-window -v -t "$SESSION" -c "$PROJECT_DIR"
tmux send-keys -t "$SESSION" "cd $PROJECT_DIR && ~/Projects/LiveAITerminal/live-ai-terminal run -a claude,codex -p 'Monitoring session for $PROJECT_DIR' --max-turns 0 --tui 2>/dev/null || echo 'Monitoring ready — use this pane for coordination'" Enter

# Layout: top split, bottom full width
tmux select-layout -t "$SESSION" main-horizontal

# Focus on Claude pane
tmux select-pane -t "$SESSION":0.0

# Attach
echo "Starting AI collaboration session for: $PROJECT_DIR"
echo "  Pane 0 (top-left):  Claude Code"
echo "  Pane 1 (top-right): Codex CLI"
echo "  Pane 2 (bottom):    Monitoring"
echo ""
echo "Controls: Ctrl+B then arrow keys to switch panes"

tmux attach -t "$SESSION"
