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
    │   ├── new-task           # 태스크 생성
    │   ├── handle-task        # 태스크 처리 (worktree 생성, agent 시작)
    │   ├── end-task           # 태스크 종료 (^x)
    │   ├── attach             # 태스크 재연결
    │   ├── cleanup            # 정리 스크립트 (/done에서 사용)
    │   ├── quick-task         # 빠른 태스크 큐 추가 (^⌥a)
    │   ├── popup-shell        # 팝업 쉘 (^a로 토글)
    │   ├── process-queue      # 큐 처리 (태스크 완료 후 자동 실행)
    │   └── _common.sh         # 공통 유틸리티 (상수, 함수)
    └── claude/commands/       # slash commands
        ├── commit.md          # /commit - 스마트 커밋
        ├── test.md            # /test - 테스트 실행
        ├── pr.md              # /pr - PR 생성
        ├── merge.md           # /merge - 브랜치 머지
        ├── finish.md          # /finish - 태스크 완료
        └── done.md            # /done - 태스크 정리

{any-project}/                 # 사용자 프로젝트 (git 또는 일반 디렉토리)
└── .taw/                      # taw가 생성하는 디렉토리
    ├── PROMPT.md              # 프로젝트별 프롬프트
    ├── .global-prompt         # -> 전역 프롬프트 (symlink, git 모드에 따라 다름)
    ├── .is-git-repo           # git 모드 마커 (git 레포일 때만 존재)
    ├── .claude                # -> _taw/claude (symlink)
    ├── .metadata/             # 로그 및 상태
    ├── .queue/                # 빠른 태스크 큐 (^⌥a로 추가)
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
| `/finish` | 태스크 완료 (commit → PR → 상태 업데이트) |
| `/done` | 태스크 정리 (worktree, branch, 디렉토리, window) |

### 불완전한 태스크 자동 재오픈

태스크가 완료되지 않은 상태(done 처리되지 않음)에서 window가 닫히거나 tmux 세션이 종료된 경우, 다음에 `taw`를 실행하면 자동으로 해당 태스크들의 window를 다시 열어줍니다.

- 새 세션 시작 시와 기존 세션 재연결 시 모두 자동으로 감지
- Claude에 새로운 입력을 보내지 않고 이전 상태 그대로 복원
- 수동으로 이어서 작업할 수 있도록 준비됨

### Window 상태

- 🤖 작업 중
- 💬 대기 중
- ✅ 완료

## 설정

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
| 팝업 쉘 | `^a` (현재 worktree에서 쉘 열기/닫기) |
| 새 태스크 | `^n` |
| 태스크 종료 | `^x` (worktree/branch 정리 및 창 닫기) |
| 빠른 태스크 큐 추가 | `^⌥a` (현재 태스크 완료 후 자동 처리) |
| Pane 이동 | `⌥ + ←/→/↑/↓` |
| Pane 분할 | `⌥ + h/j/k/l` (좌/하/상/우) |
| Pane 닫기 | `⌥ + x` |
| Window 이동 | `⇧⌥ + ←/→` |
| 도움말 | `⌥ + /` |
| Session 나가기 | `^q` (detach) |

## 빠른 태스크 큐

작업 중에 떠오른 아이디어나 추가 작업을 빠르게 큐에 추가할 수 있습니다.

1. `^⌥a`를 누르면 팝업이 열립니다
2. 태스크 내용을 입력하고 Enter
3. 현재 태스크가 완료(`/finish` 또는 `/done`)되면 큐에 있는 태스크가 자동으로 시작됩니다

큐 관리:
```bash
.taw/.queue/      # 큐 디렉토리
└── 001.task      # 대기 중인 태스크 파일
```
