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
    ├── PROMPT.md              # 전역 에이전트 프롬프트
    ├── bin/                   # 실행 파일
    │   ├── taw                # 메인 명령어 (세션 시작)
    │   └── new-task           # 태스크 생성
    └── claude/commands/       # slash commands
        ├── pr.md              # /pr - PR 생성
        └── done.md            # /done - 태스크 정리

{any-project}/                 # 사용자 프로젝트 (git 또는 일반 디렉토리)
└── .taw/                      # taw가 생성하는 디렉토리
    ├── PROMPT.md              # 프로젝트별 프롬프트
    ├── .global-prompt         # -> 전역 프롬프트 (symlink, git 모드에 따라 다름)
    ├── .is-git-repo           # git 모드 마커 (git 레포일 때만 존재)
    ├── .claude                # -> _taw/claude (symlink)
    ├── .metadata/             # 로그 및 상태
    └── agents/{task-name}/    # 태스크별 작업 공간
        ├── task               # 태스크 내용
        ├── log                # 진행 로그
        ├── attach             # 태스크 재연결 스크립트
        ├── origin             # -> 프로젝트 루트 (symlink)
        ├── worktree/          # git worktree (git 모드에서만 자동 생성)
        ├── .tab-created       # 탭 생성 마커
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
| `/pr` | PR 자동 생성 및 브라우저 열기 |
| `/merge` | worktree 브랜치를 프로젝트의 현재 브랜치에 머지 |
| `/done` | 태스크 정리 (worktree, branch, 디렉토리, window) |

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
| 새 태스크 | `^n` |
| 태스크 종료 | `^e` (agent에 /done 전송) |
| Pane 이동 | `⌥ + ←/→/↑/↓` |
| Pane 분할 | `⌥ + h/j/k/l` (좌/하/상/우) |
| Pane 닫기 | `⌥ + x` |
| Window 이동 | `⇧⌥ + ←/→` |
| 도움말 | `⌥ + /` |
| Session 나가기 | `^q` (detach) |
