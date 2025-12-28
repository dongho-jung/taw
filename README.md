# TAW (Tmux + Agent + Worktree)

Claude Code 기반의 프로젝트 관리 시스템입니다.

## 개요

- 아무 디렉토리에서 `taw` 명령어로 tmux 세션 기반의 작업 환경을 시작합니다.
- 태스크를 생성하면 자동으로 Claude Code agent가 시작됩니다.
- **Git 모드**: git 레포에서 실행 시 태스크마다 worktree 자동 생성
- **Non-Git 모드**: git 없이도 사용 가능 (worktree 없이 프로젝트 디렉토리에서 작업)

## 설치

### Go 버전 (권장)

```bash
# 빌드
make build

# 설치 (~/.local/bin)
make install

# 또는 go install로 직접 설치
go install github.com/donghojung/taw@latest
```

> **Note (macOS)**: `make install`은 자동으로 `xattr -cr` 및 `codesign -fs -`를 수행하여 `zsh: killed` 오류를 방지합니다.

## 디렉토리 구조

```
taw/                           # 이 레포
├── cmd/taw/                   # Go 메인 패키지
├── internal/                  # Go 내부 패키지
│   ├── app/                   # 애플리케이션 컨텍스트
│   ├── claude/                # Claude API 클라이언트
│   ├── config/                # 설정 관리
│   ├── constants/             # 상수 정의
│   ├── embed/                 # 임베디드 에셋
│   │   └── assets/            # HELP.md (도움말)
│   ├── git/                   # Git/Worktree 관리
│   ├── github/                # GitHub API 클라이언트
│   ├── logging/               # 로깅
│   ├── task/                  # 태스크 관리
│   ├── tmux/                  # Tmux 클라이언트
│   └── tui/                   # 터미널 UI (로그 뷰어)
├── _taw/                      # 리소스 파일 (프로젝트에 symlink로 연결됨)
│   ├── PROMPT.md              # 시스템 프롬프트 (git mode)
│   ├── PROMPT-nogit.md        # 시스템 프롬프트 (non-git mode)
│   └── claude/commands/       # slash commands (/commit, /test, /pr, /merge)
├── Makefile                   # 빌드 스크립트
└── go.mod                     # Go 모듈 파일

{any-project}/                 # 사용자 프로젝트 (git 또는 일반 디렉토리)
└── .taw/                      # taw가 생성하는 디렉토리
    ├── config                 # 프로젝트 설정 (YAML, 초기 설정 시 생성)
    ├── log                    # 통합 로그 (모든 스크립트의 로그가 여기에)
    ├── PROMPT.md              # 프로젝트별 프롬프트
    ├── .global-prompt         # -> 전역 프롬프트 (symlink, git 모드에 따라 다름)
    ├── .is-git-repo           # git 모드 마커 (git 레포일 때만 존재)
    ├── .claude                # -> _taw/claude (symlink)
    ├── .queue/                # 빠른 태스크 큐 (⌥ u로 추가)
    │   └── 001.task           # 대기 중인 태스크 (순서대로 처리)
    ├── history/               # 태스크 히스토리 저장 디렉토리
    │   └── YYMMDD_HHMMSS_task-name  # 태스크 종료 시 agent pane 캡처
    └── agents/{task-name}/    # 태스크별 작업 공간
        ├── task               # 태스크 내용
        ├── end-task           # 태스크별 end-task 스크립트 (Claude가 auto-merge 시 호출)
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

추가 태스크 생성이 필요하면 tmux 세션 내에서 `⌥ n`을 누릅니다:
- 에디터가 열리고 태스크 내용을 작성합니다
- 저장하고 종료하면 자동으로 agent가 시작됩니다
- vi/vim/nvim 사용 시 자동으로 insert 모드로 시작합니다

### Slash Commands

Agent가 사용할 수 있는 slash commands:

| Command | 설명 |
|---------|------|
| `/commit` | 스마트 커밋 (diff 분석 후 메시지 자동 생성) |
| `/test` | 프로젝트 테스트 자동 감지 및 실행 |
| `/pr` | PR 자동 생성 및 브라우저 열기 |
| `/merge` | worktree 브랜치를 프로젝트의 현재 브랜치에 머지 |

**태스크 종료**:
- `auto-merge` 모드: 태스크 완료 시 **자동으로** 커밋 → 머지 → 정리 → window 닫기 (⌥e 불필요)
- 다른 모드: `⌥ e`를 누르면 ON_COMPLETE 설정에 따라 커밋 → PR/머지 → 정리 수행

### 불완전한 태스크 자동 재오픈

태스크가 완료되지 않은 상태(`⌥ e`로 종료되지 않음)에서 window가 닫히거나 tmux 세션이 종료된 경우, 다음에 `taw`를 실행하면 자동으로 해당 태스크들의 window를 다시 열어줍니다.

- 새 세션 시작 시와 기존 세션 재연결 시 모두 자동으로 감지
- Claude에 새로운 입력을 보내지 않고 이전 상태 그대로 복원
- 수동으로 이어서 작업할 수 있도록 준비됨

### 머지된 태스크 자동 정리

외부에서 머지된 태스크(PR 머지, 브랜치 직접 머지 등)는 `taw` 실행 시 자동으로 정리됩니다.

- PR이 GitHub에서 머지된 경우 감지
- 브랜치가 main에 머지된 경우 감지
- 브랜치가 외부에서 삭제된 경우 감지
- worktree, 브랜치, 에이전트 디렉토리 자동 정리
- 열려있는 window도 자동으로 닫힘
- 정리된 태스크는 `✅ Cleaned up merged task: <task-name>` 메시지로 표시

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
🚀 TAW Setup Wizard
Work Mode:
  1. worktree (Recommended) - Each task gets its own git worktree
  2. main - All tasks work on current branch

Select [1-2, default: 1]:

On Complete Action:
  1. confirm (Recommended) - Ask before each action
  2. auto-commit - Automatically commit changes
  3. auto-merge - Auto commit + merge + cleanup
  4. auto-pr - Auto commit + create pull request

Select [1-4, default: 1]:

✅ Configuration saved!
   Work mode: worktree
   On complete: confirm
```

설정은 `.taw/config` 파일에 저장됩니다.

### 설정 재실행

```bash
taw setup  # 설정 마법사 다시 실행
```

### 설정 파일 (.taw/config)

```
# TAW Configuration
# Generated by taw setup

# Work mode: worktree or main
# - worktree: Each task gets its own git worktree (recommended)
# - main: All tasks work on the current branch
work_mode: worktree

# On complete action: confirm, auto-commit, auto-merge, or auto-pr
# - confirm: Ask before each action (recommended)
# - auto-commit: Automatically commit changes
# - auto-merge: Auto commit + merge + cleanup + close window
# - auto-pr: Auto commit + create pull request
on_complete: confirm
```

### 설정 옵션

| 설정 | 옵션 | 설명 |
|------|------|------|
| `work_mode` | `worktree` | 태스크마다 git worktree 생성 (격리, 권장) |
|             | `main` | 현재 브랜치에서 직접 작업 (단순) |
| `on_complete` | `confirm` | 각 작업 전 확인 (안전) |
|               | `auto-commit` | 자동 커밋 (머지/PR은 수동) |
|               | `auto-merge` | **태스크 완료 시 자동** 커밋 + 머지 + 정리 + window 닫기 (⌥e 불필요) |
|               | `auto-pr` | 자동 커밋 + PR 생성 (팀 협업용) |

### 기타 설정

- `_taw/PROMPT.md`: 시스템 프롬프트 (프로젝트 `.taw/.global-prompt`로 symlink됨)
- `.taw/PROMPT.md`: 프로젝트별 프롬프트 (각 프로젝트 내)
- `_taw/claude/commands/`: slash commands (프로젝트 `.taw/.claude`로 symlink됨)
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
| new window 토글 | `⌥ n` (task ↔ new window) |
| 태스크 완료 | `⌥ e` (user pane에서 진행상황 표시, commit → PR/merge → cleanup) |
| 완료 태스크 일괄 머지 | `⌥ m` (✅ 상태 태스크 모두 merge + end) |
| 쉘 pane 토글 | `⌥ p` (하단 40%, 현재 worktree에서 쉘 열기/닫기) |
| 실시간 로그 | `⌥ l` (로그 뷰어 토글, vim-like 네비게이션 지원) |
| 빠른 태스크 큐 추가 | `⌥ u` (현재 태스크 완료 후 자동 처리) |
| 도움말 | `⌥ /` |
| Session 나가기 | `⌥ q` (detach) |

## 빠른 태스크 큐

작업 중에 떠오른 아이디어나 추가 작업을 빠르게 큐에 추가할 수 있습니다.

1. `⌥ u`를 누르면 팝업이 열립니다
2. 태스크 내용을 입력하고 Enter
3. 현재 태스크가 완료(`⌥ e`)되면 큐에 있는 태스크가 자동으로 시작됩니다

큐 관리:
```bash
.taw/.queue/      # 큐 디렉토리
└── 001.task      # 대기 중인 태스크 파일
```

## 로그 뷰어

`⌥ l`을 누르면 실시간 로그 뷰어가 팝업으로 열립니다.

### 조작법

| 키 | 설명 |
|----|------|
| `↑` / `↓` | 세로 스크롤 |
| `←` / `→` | 가로 스크롤 (word wrap 꺼져있을 때) |
| `g` | 맨 위로 이동 |
| `G` | 맨 아래로 이동 |
| `PgUp` / `PgDn` | 페이지 단위 스크롤 |
| `s` | Tail 모드 토글 (새 로그 자동 추적) |
| `w` | Word Wrap 토글 |
| `q` / `Esc` / `⌥ l` | 로그 뷰어 닫기 |

### 상태 표시

로그 뷰어 하단에 현재 상태가 표시됩니다:
- `[TAIL]` - Tail 모드 활성화 (새 로그 자동 추적)
- `[WRAP]` - Word Wrap 모드 활성화

### 로그 형식

로그는 상세한 정보를 포함합니다:

```
[25-12-28 03:49:45.0] [INFO ] [handle-task:my-task] [RunE] Task started successfully
```

형식: `[timestamp] [level] [script:task] [caller] message`

- **timestamp**: YY-MM-DD HH:MM:SS.s (십분의 일초 단위)
- **level**: INFO, WARN, ERROR, DEBUG
- **script:task**: 스크립트와 태스크 컨텍스트
- **caller**: 로그를 남긴 함수명
- **message**: 로그 메시지 (key=value 형태의 상세 정보 포함)

시간이 걸리는 작업은 시작/완료 로그와 함께 소요 시간이 기록됩니다:
```
[25-12-28 03:49:45.0] [INFO ] [handle-task:my-task] [RunE] worktree setup started
[25-12-28 03:49:45.2] [INFO ] [handle-task:my-task] [RunE] worktree setup completed in 153ms: branch=my-task, path=/path/to/worktree
```

## 태스크 히스토리

태스크가 종료될 때 agent pane의 전체 내용이 자동으로 캡처되어 저장됩니다.

### 저장 위치

```
.taw/history/
└── YYMMDD_HHMMSS_task-name  # 예: 241228_134501_my-feature
```

### 활용 예시

- 이전 태스크에서 agent가 수행한 작업 확인
- 문제 해결 과정 추적
- 학습 및 개선을 위한 참고 자료
