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
# Spinner (Loading Indicator)
# ============================================================================

# Global variable to track spinner PID
SPINNER_PID=""

# Start a spinner with message
# Usage: start_spinner "Processing..."
# Note: Caller should ensure stop_spinner is called on exit (e.g., in a trap)
start_spinner() {
    local message="${1:-Working...}"
    local delay=0.1
    local frames=('⠋' '⠙' '⠹' '⠸' '⠼' '⠴' '⠦' '⠧' '⠇' '⠏')

    # Run spinner in background
    (
        local i=0
        while true; do
            printf "\r%s %s" "${frames[i % ${#frames[@]}]}" "$message"
            sleep "$delay"
            i=$((i + 1))
        done
    ) &
    SPINNER_PID=$!
}

# Stop the spinner
# Usage: stop_spinner
stop_spinner() {
    if [ -n "$SPINNER_PID" ] && kill -0 "$SPINNER_PID" 2>/dev/null; then
        kill "$SPINNER_PID" 2>/dev/null
        wait "$SPINNER_PID" 2>/dev/null || true
        printf "\r\033[K"  # Clear the spinner line
        SPINNER_PID=""
    fi
}

# ============================================================================
# Configuration
# ============================================================================

# Default configuration values
readonly DEFAULT_WORK_MODE="worktree"           # worktree | main
readonly DEFAULT_ON_COMPLETE="confirm"          # auto-commit | auto-merge | auto-pr | confirm

# Get config file path
# Usage: config_file=$(get_config_path "$TAW_DIR")
get_config_path() {
    local taw_dir="$1"
    echo "$taw_dir/config"
}

# Read a config value (simple YAML parser for key: value format)
# Usage: value=$(read_config "$TAW_DIR" "work_mode" "default_value")
read_config() {
    local taw_dir="$1"
    local key="$2"
    local default="$3"
    local config_file="$taw_dir/config"

    if [ ! -f "$config_file" ]; then
        echo "$default"
        return 0
    fi

    # Parse YAML: find key, extract value, strip quotes and whitespace
    local value=$(grep -E "^${key}:" "$config_file" 2>/dev/null | head -1 | cut -d':' -f2- | sed 's/^[[:space:]]*//;s/[[:space:]]*$//' | sed 's/^["'"'"']//;s/["'"'"']$//')

    if [ -z "$value" ]; then
        echo "$default"
    else
        echo "$value"
    fi
}

# Check if config exists
# Usage: if config_exists "$TAW_DIR"; then ...
config_exists() {
    local taw_dir="$1"
    [ -f "$taw_dir/config" ]
}


# ============================================================================
# Corrupted Worktree Detection
# ============================================================================

# Check if a worktree is corrupted/invalid
# Returns: "ok", "missing_worktree", "not_in_git", "invalid_git", "missing_branch"
# Usage: status=$(check_worktree_status "$PROJECT_DIR" "$WORKTREE_DIR" "$TASK_NAME")
check_worktree_status() {
    local project_dir="$1"
    local worktree_dir="$2"
    local task_name="$3"

    # Check if worktree directory exists
    if [ ! -d "$worktree_dir" ]; then
        echo "missing_worktree"
        return 0
    fi

    # Check if worktree has valid .git file
    if [ ! -f "$worktree_dir/.git" ]; then
        echo "invalid_git"
        return 0
    fi

    # Check if worktree is registered in git
    if ! git -C "$project_dir" worktree list 2>/dev/null | grep -q "$worktree_dir"; then
        echo "not_in_git"
        return 0
    fi

    # Check if branch exists
    if ! git -C "$project_dir" rev-parse --verify "$task_name" &>/dev/null; then
        echo "missing_branch"
        return 0
    fi

    echo "ok"
}

# Find corrupted tasks: tasks where worktree state is invalid
# Usage: corrupted_tasks=$(find_corrupted_tasks "$AGENTS_DIR" "$PROJECT_DIR" "$SESSION_NAME")
# Returns: newline-separated list of "task_dir|status" pairs
find_corrupted_tasks() {
    local agents_dir="$1"
    local project_dir="$2"
    local session_name="${3:-}"
    local result=""

    [ -d "$agents_dir" ] || return 0

    for agent_dir in "$agents_dir"/*/; do
        [ -d "$agent_dir" ] || continue

        local task_name=$(basename "$agent_dir")
        local worktree_dir="$agent_dir/worktree"
        local tab_lock="$agent_dir/.tab-lock"

        # Skip tasks that have an active window (if session_name provided)
        if [ -n "$session_name" ] && [ -d "$tab_lock" ] && [ -f "$tab_lock/window_id" ]; then
            local window_id=$(cat "$tab_lock/window_id")
            # Check if window actually exists
            if tmux -L "taw-$session_name" list-windows -F "#{window_id}" 2>/dev/null | grep -q "^${window_id}$"; then
                # Window still exists, skip this task
                continue
            fi
        fi

        # Check worktree status (only for git mode tasks with worktree dir or task file)
        if [ -d "$worktree_dir" ] || [ -f "$agent_dir/task" ]; then
            local status=$(check_worktree_status "$project_dir" "$worktree_dir" "$task_name")
            if [ "$status" != "ok" ]; then
                if [ -n "$result" ]; then
                    result="$result"$'\n'"$agent_dir|$status"
                else
                    result="$agent_dir|$status"
                fi
            fi
        fi
    done

    echo "$result"
}

# Get human-readable description of worktree status
# Usage: desc=$(get_worktree_status_description "$status")
get_worktree_status_description() {
    local status="$1"
    case "$status" in
        missing_worktree)
            echo "worktree 디렉토리가 없습니다 (외부에서 삭제됨)"
            ;;
        not_in_git)
            echo "worktree가 git에 등록되어 있지 않습니다 (외부에서 정리됨)"
            ;;
        invalid_git)
            echo "worktree의 .git 파일이 손상되었습니다"
            ;;
        missing_branch)
            echo "branch가 없습니다 (외부에서 삭제됨)"
            ;;
        *)
            echo "알 수 없는 상태: $status"
            ;;
    esac
}

# ============================================================================
# Merged Task Detection
# ============================================================================

# Check if a task's branch is merged (either to main or via PR)
# Usage: is_merged=$(check_task_merged "$PROJECT_DIR" "$TASK_NAME" "$AGENT_DIR")
# Returns: "merged" or "not_merged"
check_task_merged() {
    local project_dir="$1"
    local task_name="$2"
    local agent_dir="$3"

    # Check if branch exists first
    if ! git -C "$project_dir" rev-parse --verify "$task_name" &>/dev/null; then
        # Branch doesn't exist - could have been deleted after merge
        echo "merged"
        return 0
    fi

    # Check if PR exists and is merged
    local pr_file="$agent_dir/.pr"
    if [ -f "$pr_file" ]; then
        local pr_number=$(cat "$pr_file")
        local pr_state=$(gh pr view "$pr_number" --json merged -q '.merged' 2>/dev/null || echo "false")
        if [ "$pr_state" = "true" ]; then
            echo "merged"
            return 0
        fi
    fi

    # Check if branch is merged to main
    git -C "$project_dir" fetch origin main 2>/dev/null || true
    if git -C "$project_dir" branch --merged main 2>/dev/null | grep -q "\\b$task_name\\b"; then
        echo "merged"
        return 0
    fi

    echo "not_merged"
}

# Find merged tasks: tasks where branch has been merged but not cleaned up
# Usage: merged_tasks=$(find_merged_tasks "$AGENTS_DIR" "$PROJECT_DIR" "$SESSION_NAME")
# Returns: newline-separated list of agent directories
find_merged_tasks() {
    local agents_dir="$1"
    local project_dir="$2"
    local session_name="${3:-}"
    local result=""

    [ -d "$agents_dir" ] || return 0

    for agent_dir in "$agents_dir"/*/; do
        [ -d "$agent_dir" ] || continue

        local task_name=$(basename "$agent_dir")
        local tab_lock="$agent_dir/.tab-lock"

        # Skip tasks that have an active window (if session_name provided)
        if [ -n "$session_name" ] && [ -d "$tab_lock" ] && [ -f "$tab_lock/window_id" ]; then
            local window_id=$(cat "$tab_lock/window_id")
            # Check if window actually exists
            if tmux -L "taw-$session_name" list-windows -F "#{window_id}" 2>/dev/null | grep -q "^${window_id}$"; then
                # Window still exists, skip this task
                continue
            fi
        fi

        # Check if task is merged
        local status=$(check_task_merged "$project_dir" "$task_name" "$agent_dir")
        if [ "$status" = "merged" ]; then
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
