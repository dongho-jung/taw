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

# Task name constraints
readonly TASK_NAME_MIN_LEN=8
readonly TASK_NAME_MAX_LEN=32

# Git defaults
readonly DEFAULT_MAIN_BRANCH="main"

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

# Global log context (set by each script)
# TAW_LOG_SCRIPT: script name (e.g., "taw", "handle-task")
# TAW_LOG_TASK: current task name (optional)
# TAW_LOG_FILE: unified log file path

# Initialize logging for a script
# Usage: init_logging "$TAW_DIR" "script-name" ["task-name"]
init_logging() {
    local taw_dir="$1"
    local script_name="$2"
    local task_name="${3:-}"

    export TAW_LOG_FILE="$taw_dir/log"
    export TAW_LOG_SCRIPT="$script_name"
    export TAW_LOG_TASK="$task_name"
}

# Debug output (uses TAW_DEBUG env var)
# Usage: debug "message"
debug() {
    if [ "${TAW_DEBUG:-0}" = "1" ]; then
        local script_name="${TAW_LOG_SCRIPT:-$(basename "${BASH_SOURCE[1]}")}"
        echo "[DEBUG $script_name] $*" >&2
    fi
}

# Log to unified file with timestamp, script name, and task
# Usage: log "message"
# Format: [2024-12-27 10:30:45] [handle-task:fix-bug] Processing task
log() {
    local message="$1"
    local log_file="${TAW_LOG_FILE:-}"

    if [ -n "$log_file" ]; then
        local script_name="${TAW_LOG_SCRIPT:-unknown}"
        local task_name="${TAW_LOG_TASK:-}"
        local context

        if [ -n "$task_name" ]; then
            context="$script_name:$task_name"
        else
            context="$script_name"
        fi

        echo "[$(date '+%Y-%m-%d %H:%M:%S')] [$context] $message" >> "$log_file"
    fi
}

# Log warning (both to file and stderr)
# Usage: warn "message"
warn() {
    local message="$1"
    echo "${EMOJI_WARNING} $message" >&2
    log "WARNING: $message"
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

        # Note: We don't skip tasks with active windows here.
        # auto_cleanup_merged_tasks() will close the window before cleanup.

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
# Git Helpers
# ============================================================================

# Get the main branch name for a repository
# Usage: main_branch=$(get_main_branch "$PROJECT_DIR")
# Returns: main branch name (defaults to "main" if can't detect)
get_main_branch() {
    local project_dir="$1"
    local branch

    # Try to get from remote HEAD reference
    branch=$(git -C "$project_dir" symbolic-ref refs/remotes/origin/HEAD 2>/dev/null | sed 's@^refs/remotes/origin/@@')

    if [ -z "$branch" ]; then
        # Fallback: check for common branch names
        for candidate in main master; do
            if git -C "$project_dir" rev-parse --verify "$candidate" &>/dev/null; then
                branch="$candidate"
                break
            fi
        done
    fi

    echo "${branch:-$DEFAULT_MAIN_BRANCH}"
}

# Check if current directory is a git repo root
# Usage: if is_git_repo_root "$dir"; then ...
is_git_repo_root() {
    local dir="$1"
    local git_root

    git_root=$(git -C "$dir" rev-parse --show-toplevel 2>/dev/null) || return 1

    local dir_real git_root_real
    dir_real=$(cd "$dir" && pwd -P)
    git_root_real=$(cd "$git_root" && pwd -P)

    [ "$dir_real" = "$git_root_real" ]
}

# ============================================================================
# Project Directory Discovery
# ============================================================================

# Find project directory by walking up from a path until .taw is found
# Usage: project_dir=$(find_project_dir "$path")
# Returns: Project directory path, or empty string if not found
find_project_dir_from_path() {
    local dir="$1"
    while [ "$dir" != "/" ]; do
        if [ -d "$dir/.taw" ]; then
            echo "$dir"
            return 0
        fi
        dir=$(dirname "$dir")
    done
    return 1
}

# Find project directory from tmux session name
# Usage: project_dir=$(find_project_dir_from_session "$session_name")
# Returns: Project directory path, or empty string if not found
find_project_dir_from_session() {
    local session="$1"
    local pane_path
    pane_path=$(tmux -L "taw-$session" list-panes -s -F '#{pane_current_path}' 2>/dev/null | head -1)

    if [ -n "$pane_path" ]; then
        find_project_dir_from_path "$pane_path"
    else
        return 1
    fi
}

# ============================================================================
# Claude Interaction Helpers
# ============================================================================

# Get content from a tmux pane
# Usage: content=$(get_pane_content "$session_name" "$window_id" "$pane_id")
get_pane_content() {
    local session="$1"
    local window_id="$2"
    local pane_id="${3:-0}"
    tmux -L "taw-$session" capture-pane -t "${window_id}.${pane_id}" -p 2>/dev/null || echo ""
}

# Send a shell command to a pane (with Enter key)
# Usage: send_shell_cmd "$session_name" "$window_id" "$command"
send_shell_cmd() {
    local session="$1"
    local window_id="$2"
    local cmd="$3"
    tmux -L "taw-$session" send-keys -t "${window_id}.0" -l "$cmd"
    tmux -L "taw-$session" send-keys -t "${window_id}.0" Enter
}

# Send input to Claude Code (Escape + CR for multiline submit)
# Usage: send_to_claude "$session_name" "$window_id" "$input"
send_to_claude() {
    local session="$1"
    local window_id="$2"
    local input="$3"

    sleep 0.3
    tmux -L "taw-$session" send-keys -t "${window_id}.0" -l "$input"
    sleep 0.2
    tmux -L "taw-$session" send-keys -t "${window_id}.0" Escape
    sleep 0.2
    tmux -L "taw-$session" send-keys -t "${window_id}.0" -H 0d  # ASCII 13 (Carriage Return)
    debug "Sent input to claude: ${input:0:50}..."
}

# Wait for Claude to be ready (poll until expected output is seen)
# Usage: wait_for_claude_ready "$session_name" "$window_id" [max_attempts]
# Returns: 0 if ready, 1 if timeout
wait_for_claude_ready() {
    local session="$1"
    local window_id="$2"
    local max_attempts="${3:-$CLAUDE_READY_MAX_ATTEMPTS}"
    local attempt=0

    debug "Waiting for claude to be ready (max $max_attempts attempts)..."
    while [ $attempt -lt $max_attempts ]; do
        local content
        content=$(get_pane_content "$session" "$window_id")

        # Claude is ready when we see trust prompt, input prompt, or bypass permissions
        if echo "$content" | grep -qE "Trust|trust|╭─|^> $|bypass permissions"; then
            debug "Claude ready after $attempt attempts"
            return 0
        fi

        # Show progress every 10 attempts in debug mode
        if [ "${TAW_DEBUG:-0}" = "1" ] && [ $((attempt % 10)) -eq 0 ]; then
            debug "Attempt $attempt: waiting for claude..."
        fi

        sleep "$CLAUDE_READY_POLL_INTERVAL"
        attempt=$((attempt + 1))
    done

    debug "TIMEOUT waiting for claude after $max_attempts attempts"
    return 1
}

# ============================================================================
# Merge Conflict Resolution
# ============================================================================

# Try to auto-resolve merge conflicts
# Returns: 0 if resolved, 1 if unresolvable, 2 if no conflicts
# Usage: try_auto_resolve_conflicts "$PROJECT_DIR"
try_auto_resolve_conflicts() {
    local project_dir="$1"

    cd "$project_dir"

    # Check if there are actual conflicts
    local conflict_files
    conflict_files=$(git diff --name-only --diff-filter=U 2>/dev/null)

    if [ -z "$conflict_files" ]; then
        return 2  # No conflicts
    fi

    debug "Conflict files: $conflict_files"

    # Try to auto-resolve each conflicted file
    local unresolved_count=0
    local resolved_count=0

    while IFS= read -r file; do
        [ -z "$file" ] && continue

        # Check if conflict markers exist
        if grep -q "^<<<<<<< " "$file" 2>/dev/null; then
            # Check if it's a simple conflict (only whitespace or formatting changes)
            # For now, we'll try accepting both changes with git checkout --merge

            # Strategy 1: Try to accept ours for auto-generated files
            if [[ "$file" == *.lock || "$file" == *package-lock.json || "$file" == *yarn.lock ]]; then
                git checkout --ours "$file" 2>/dev/null && git add "$file" && {
                    debug "Auto-resolved (ours) lock file: $file"
                    resolved_count=$((resolved_count + 1))
                    continue
                }
            fi

            # Strategy 2: For other files, just count as unresolved
            debug "Cannot auto-resolve: $file"
            unresolved_count=$((unresolved_count + 1))
        else
            # No conflict markers, just needs to be staged
            git add "$file" 2>/dev/null
            resolved_count=$((resolved_count + 1))
        fi
    done <<< "$conflict_files"

    if [ "$unresolved_count" -gt 0 ]; then
        return 1  # Unresolvable conflicts remain
    fi

    # All conflicts resolved, try to complete the merge
    if [ "$resolved_count" -gt 0 ]; then
        if git commit --no-edit 2>/dev/null; then
            return 0  # Successfully resolved
        fi
    fi

    return 1
}

# Get list of conflicted files with details
# Usage: conflict_info=$(get_conflict_info "$PROJECT_DIR")
get_conflict_info() {
    local project_dir="$1"

    cd "$project_dir"

    local conflict_files
    conflict_files=$(git diff --name-only --diff-filter=U 2>/dev/null)

    if [ -z "$conflict_files" ]; then
        echo "No conflicts detected"
        return 0
    fi

    local count=0
    while IFS= read -r file; do
        [ -z "$file" ] && continue
        count=$((count + 1))
    done <<< "$conflict_files"

    echo "Conflicted files ($count):"
    echo "$conflict_files" | while IFS= read -r file; do
        [ -z "$file" ] && continue
        echo "  - $file"
    done
}

# Update window status to help needed
# Usage: set_window_help_needed "$SESSION_NAME" "$WINDOW_ID" "$TASK_NAME"
set_window_help_needed() {
    local session="$1"
    local window_id="$2"
    local task_name="$3"

    local display_name="${task_name:0:12}"
    local new_name="${EMOJI_WAITING}${display_name}"

    tm "$session" rename-window -t "$window_id" "$new_name" 2>/dev/null || true
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
