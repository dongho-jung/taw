# TAW (Tmux + Agent + Worktree)

Claude Code 기반의 프로젝트 관리 시스템입니다.

## 개요

- 아무 디렉토리에서 `taw` 명령어로 tmux 세션 기반의 작업 환경을 시작합니다.
- 태스크를 생성하면 자동으로 Claude Code agent가 시작됩니다.
- **Git 모드**: git 레포에서 실행 시 태스크마다 worktree 자동 생성
- **Non-Git 모드**: git 없이도 사용 가능 (worktree 없이 프로젝트 디렉토리에서 작업)

## 설치

```bash
./install    # ~/.local/bin/taw 설치
./uninstall  # ~/.local/bin/taw 제거
```

## 디렉토리 구조

```
taw/                           # 이 레포
├── install                    # taw 설치
├── uninstall                  # taw 제거
└── _taw/                      # 전역 설정
    ├── PROMPT.md              # 전역 에이전트 프롬프트 (git 모드)
    ├── PROMPT-nogit.md        # 전역 에이전트 프롬프트 (non-git 모드)
    ├── HELP.md                # 도움말 (⌥ + / 로 열람)
    ├── bin/                   # 실행 파일
    │   ├── taw                # 메인 명령어 (세션 시작)
    │   ├── setup              # 초기 설정 마법사
    │   ├── new-task           # 태스크 생성
    │   ├── handle-task        # 태스크 처리 (worktree 생성, agent 시작)
    │   ├── end-task           # 태스크 종료 (⌥ e)
    │   ├── attach             # 태스크 재연결
    │   ├── cleanup            # 정리 스크립트 (/done에서 사용)
    │   ├── quick-task         # 빠른 태스크 큐 추가 (⌥ u)
    │   ├── popup-shell        # 팝업 쉘 (⌥p로 열고, ⌥q로 닫기)
    │   ├── process-queue      # 큐 처리 (태스크 완료 후 자동 실행)
    │   ├── recover-task       # 손상된 태스크 복구/정리
    │   └── _common.sh         # 공통 유틸리티 (상수, 함수, 설정)
    └── claude/commands/       # slash commands
        ├── commit.md          # /commit - 스마트 커밋
        ├── test.md            # /test - 테스트 실행
        ├── pr.md              # /pr - PR 생성
        ├── merge.md           # /merge - 브랜치 머지
        ├── finish.md          # /finish - 태스크 완료
        └── done.md            # /done - 태스크 정리

{any-project}/                 # 사용자 프로젝트 (git 또는 일반 디렉토리)
└── .taw/                      # taw가 생성하는 디렉토리
    ├── config                 # 프로젝트 설정 (YAML, 초기 설정 시 생성)
    ├── PROMPT.md              # 프로젝트별 프롬프트
    ├── .global-prompt         # -> 전역 프롬프트 (symlink, git 모드에 따라 다름)
    ├── .is-git-repo           # git 모드 마커 (git 레포일 때만 존재)
    ├── .claude                # -> _taw/claude (symlink)
    ├── .metadata/             # 로그 및 상태
    ├── .queue/                # 빠른 태스크 큐 (⌥ u로 추가)
    │   └── 001.task           # 대기 중인 태스크 (순서대로 처리)
    └── agents/{task-name}/    # 태스크별 작업 공간
        ├── task               # 태스크 내용
        ├── log                # 진행 로그
        ├── attach             # 태스크 재연결 스크립트
        ├── origin             # -> 프로젝트 루트 (symlink)
        ├── worktree/          # git worktree (git 모드에서만 자동 생성)
        ├── .tab-lock/         # 탭 생성 락 (atomic mkdir로 race condition 방지)
        │   └── window_id      # tmux window ID (cleanup에서 사용)
        └── .pr                # PR 번호 (생성 시)
```

## 사용법

### 프로젝트에서 taw 시작

```bash
cd /path/to/your/project  # git 레포 또는 일반 디렉토리
taw  # .taw 디렉토리 생성 및 tmux 세션 시작 → 자동으로 new-task 실행
```

- git 레포에서 실행: Git 모드 (worktree 자동 생성)
- 일반 디렉토리에서 실행: Non-Git 모드 (프로젝트 디렉토리에서 직접 작업)

첫 시작 시 자동으로 태스크 작성 에디터가 열립니다.

### 태스크 생성

추가 태스크 생성이 필요하면 tmux 세션 내에서:
```bash
.taw/new-task  # $EDITOR에서 태스크 작성 → 자동으로 agent 시작
```

vi/vim/nvim 사용 시 자동으로 insert 모드로 시작합니다.

### Slash Commands

Agent가 사용할 수 있는 slash commands:

| Command | 설명 |
|---------|------|
| `/commit` | 스마트 커밋 (diff 분석 후 메시지 자동 생성) |
| `/test` | 프로젝트 테스트 자동 감지 및 실행 |
| `/pr` | PR 자동 생성 및 브라우저 열기 |
| `/merge` | worktree 브랜치를 프로젝트의 현재 브랜치에 머지 |
| `/finish` | 태스크 완료 (commit → push → PR → 상태 업데이트) |
| `/done` | 태스크 정리 (worktree, branch, 디렉토리, window) |

### 불완전한 태스크 자동 재오픈

태스크가 완료되지 않은 상태(done 처리되지 않음)에서 window가 닫히거나 tmux 세션이 종료된 경우, 다음에 `taw`를 실행하면 자동으로 해당 태스크들의 window를 다시 열어줍니다.

- 새 세션 시작 시와 기존 세션 재연결 시 모두 자동으로 감지
- Claude에 새로운 입력을 보내지 않고 이전 상태 그대로 복원
- 수동으로 이어서 작업할 수 있도록 준비됨

### 손상된 Worktree 복구

외부에서 worktree가 삭제되거나 git 상태가 꼬인 경우, `taw`를 실행하면 자동으로 감지하여 복구 옵션을 제공합니다.

감지되는 상태:
- `missing_worktree`: worktree 디렉토리가 없음 (외부에서 삭제됨)
- `not_in_git`: worktree가 git에 등록되어 있지 않음 (외부에서 정리됨)
- `invalid_git`: worktree의 .git 파일이 손상됨
- `missing_branch`: branch가 없음 (외부에서 삭제됨)

복구 옵션:
- **Recover**: worktree를 재생성하고 작업 계속
- **Cleanup**: 태스크와 관련 리소스(worktree, branch) 정리

손상된 태스크는 ⚠️ 이모지와 함께 window가 열리고, 사용자가 복구 또는 정리를 선택할 수 있습니다.

### Window 상태

- 🤖 작업 중
- 💬 대기 중 (사용자 입력 필요)
- ✅ 완료
- ⚠️ 손상됨 (복구 또는 정리 필요)

## 설정

### 초기 설정 (Initial Setup)

처음 `taw`를 실행하면 설정 마법사가 나타납니다:

```
=== TAW Initial Setup ===

Work Mode
How should taw create working directories for tasks?

  → 1) worktree - Create git worktree per task (isolated, recommended)
    2) main     - Work directly on current branch (simpler)

Select [1-2, default: 1]:
```

설정은 `.taw/config` 파일에 YAML 형식으로 저장됩니다.

### 설정 재실행

```bash
taw setup  # 설정 마법사 다시 실행
```

### 설정 파일 (.taw/config)

```yaml
# TAW Configuration
# Edit this file directly to change settings

# Work Mode (git repositories only)
# Options: worktree | main
#   worktree - Create a git worktree per task (recommended)
#   main     - Work directly on current branch
work_mode: worktree

# On Complete Behavior
# Options: confirm | auto-commit | auto-merge | auto-pr
#   confirm     - Ask before each action (commit, merge, PR)
#   auto-commit - Automatically commit changes (manual merge/PR)
#   auto-merge  - Auto commit + merge to current branch
#   auto-pr     - Auto commit + create Pull Request (for teams)
on_complete: confirm
```

### 설정 옵션

| 설정 | 옵션 | 설명 |
|------|------|------|
| `work_mode` | `worktree` | 태스크마다 git worktree 생성 (격리, 권장) |
|             | `main` | 현재 브랜치에서 직접 작업 (단순) |
| `on_complete` | `confirm` | 각 작업 전 확인 (안전) |
|               | `auto-commit` | 자동 커밋 (머지/PR은 수동) |
|               | `auto-merge` | 자동 커밋 + 현재 브랜치에 머지 |
|               | `auto-pr` | 자동 커밋 + PR 생성 (팀 협업용) |

### 기타 설정

- `_taw/PROMPT.md`: 전역 에이전트 프롬프트
- `.taw/PROMPT.md`: 프로젝트별 프롬프트 (각 프로젝트 내)
- `_taw/claude/commands/`: slash commands
- `EDITOR` 환경변수: 태스크 작성 에디터 (기본: vim)

## 의존성

```bash
brew install tmux gh
```

## tmux 단축키

| 동작 | 단축키 |
|------|--------|
| Pane 순환 | `⌥ Tab` |
| Window 이동 | `⌥ ←/→` |
| 새 태스크 | `⌥ n` |
| 태스크 종료 | `⌥ e` (worktree/branch 정리 및 창 닫기) |
| 팝업 쉘 | `⌥ p` (현재 worktree에서 쉘 열기/닫기) |
| 빠른 태스크 큐 추가 | `⌥ u` (현재 태스크 완료 후 자동 처리) |
| Session 나가기 | `⌥ q` (detach) |
| 도움말 | `⌥ /` |

## 빠른 태스크 큐

작업 중에 떠오른 아이디어나 추가 작업을 빠르게 큐에 추가할 수 있습니다.

1. `⌥ u`를 누르면 팝업이 열립니다
2. 태스크 내용을 입력하고 Enter
3. 현재 태스크가 완료(`/finish` 또는 `/done`)되면 큐에 있는 태스크가 자동으로 시작됩니다

큐 관리:
```bash
.taw/.queue/      # 큐 디렉토리
└── 001.task      # 대기 중인 태스크 파일
```
