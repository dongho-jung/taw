#!/bin/bash

# TAW Common Utilities
# Source this file: source "$(dirname "$0")/_common.sh"

# ============================================================================
# Constants
# ============================================================================

# Window status emojis
readonly EMOJI_WORKING="🤖"
readonly EMOJI_WAITING="💬"
readonly EMOJI_DONE="✅"
readonly EMOJI_WARNING="⚠️"

# Display limits
readonly MAX_DISPLAY_NAME_LEN=32

# Claude interaction
readonly CLAUDE_READY_MAX_ATTEMPTS=60      # 60 * 0.5s = 30s max wait
readonly CLAUDE_READY_POLL_INTERVAL=0.5

# Timeouts (seconds)
readonly WORKTREE_TIMEOUT=30               # git worktree operations
readonly WINDOW_CREATION_TIMEOUT=30        # wait for tmux window creation
readonly TASK_NAME_TIMEOUTS=(3 5 10)       # retry timeouts for task name generation

# ============================================================================
# Path Utilities
# ============================================================================

# Resolve symlinks (macOS compatible)
# Usage: resolved=$(resolve_path "$path")
resolve_path() {
    local path="$1"
    while [ -L "$path" ]; do
        local dir="$(cd "$(dirname "$path")" && pwd)"
        path="$(readlink "$path")"
        [[ "$path" != /* ]] && path="$dir/$path"
    done
    echo "$(cd "$(dirname "$path")" && pwd)/$(basename "$path")"
}

# Get TAW_HOME from script location (assumes script is in _taw/bin/)
# Usage: TAW_HOME=$(get_taw_home)
get_taw_home() {
    local script_path="$(resolve_path "${BASH_SOURCE[1]}")"
    echo "$(cd "$(dirname "$script_path")/../.." && pwd)"
}

# Truncate name to max chars with ... in middle if needed
# Usage: display_name=$(truncate_name "$name" [max_len])
truncate_name() {
    local name="$1"
    local max_len="${2:-$MAX_DISPLAY_NAME_LEN}"
    local len=${#name}

    if [ $len -le $max_len ]; then
        echo "$name"
    else
        local keep=$(( (max_len - 3) / 2 ))
        local left="${name:0:$keep}"
        local right="${name: -$keep}"
        echo "${left}...${right}"
    fi
}

# ============================================================================
# Logging
# ============================================================================

# Debug output (uses TAW_DEBUG env var)
# Usage: debug "message"
debug() {
    if [ "${TAW_DEBUG:-0}" = "1" ]; then
        local script_name="$(basename "${BASH_SOURCE[1]}")"
        echo "[DEBUG $script_name] $*" >&2
    fi
}

# Log to file with timestamp
# Usage: log "message" "$LOG_FILE"
log() {
    local message="$1"
    local log_file="${2:-$LOG_FILE}"
    if [ -n "$log_file" ]; then
        echo "[$(date '+%Y-%m-%d %H:%M:%S')] $message" >> "$log_file"
    fi
}

# Log warning (both to file and stderr)
# Usage: warn "message" "$LOG_FILE"
warn() {
    local message="$1"
    local log_file="${2:-$LOG_FILE}"
    echo "${EMOJI_WARNING} $message" >&2
    log "WARNING: $message" "$log_file"
}

# ============================================================================
# Tmux Helpers
# ============================================================================

# Tmux command with project-specific socket
# Usage: tm "$SESSION_NAME" command args...
tm() {
    local session="$1"
    shift
    tmux -L "taw-$session" "$@"
}

# ============================================================================
# Incomplete Task Detection
# ============================================================================

# Find incomplete tasks: tasks where window was closed but task not done
# Usage: incomplete_tasks=$(find_incomplete_tasks "$AGENTS_DIR" "$SESSION_NAME")
# Returns newline-separated list of task directory paths
find_incomplete_tasks() {
    local agents_dir="$1"
    local session_name="$2"
    local result=""

    [ -d "$agents_dir" ] || return 0

    for agent_dir in "$agents_dir"/*/; do
        [ -d "$agent_dir" ] || continue

        local task_name=$(basename "$agent_dir")
        local tab_lock="$agent_dir/.tab-lock"
        local window_id_file="$tab_lock/window_id"

        # Skip if no .tab-lock (task never started or already cleaned)
        [ -d "$tab_lock" ] || continue

        # Skip if no window_id file
        [ -f "$window_id_file" ] || continue

        local window_id=$(cat "$window_id_file")

        # Check if window still exists in tmux
        if ! tmux -L "taw-$session_name" list-windows -F "#{window_id}" 2>/dev/null | grep -q "^${window_id}$"; then
            # Window doesn't exist but task directory does = incomplete task
            if [ -n "$result" ]; then
                result="$result"$'\n'"$agent_dir"
            else
                result="$agent_dir"
            fi
        fi
    done

    echo "$result"
}

# ============================================================================
# Task Cleanup (shared between end-task and /done)
# ============================================================================

# Clean up a task: remove worktree, branch, agent dir
# Usage: cleanup_task "$TASK_NAME" "$PROJECT_DIR" "$AGENT_DIR" "$IS_GIT_MODE"
cleanup_task() {
    local task_name="$1"
    local project_dir="$2"
    local agent_dir="$3"
    local is_git_mode="${4:-false}"
    local worktree_dir="$agent_dir/worktree"

    # Remove worktree (git mode only)
    if [ "$is_git_mode" = true ] && [ -d "$worktree_dir" ]; then
        debug "Removing worktree: $worktree_dir"
        if ! git -C "$project_dir" worktree remove "$worktree_dir" --force 2>/dev/null; then
            warn "Failed to remove worktree: $worktree_dir"
        fi
        git -C "$project_dir" worktree prune 2>/dev/null || true
    fi

    # Delete branch (git mode only)
    if [ "$is_git_mode" = true ]; then
        if git -C "$project_dir" rev-parse --verify "$task_name" &>/dev/null; then
            debug "Deleting branch: $task_name"
            if ! git -C "$project_dir" branch -D "$task_name" 2>/dev/null; then
                warn "Failed to delete branch: $task_name"
            fi
        fi
    fi

    # Remove agent directory
    if [ -d "$agent_dir" ]; then
        debug "Removing agent directory: $agent_dir"
        rm -rf "$agent_dir"
    fi
}
