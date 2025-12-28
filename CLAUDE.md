# CLAUDE.md

## 빌드 및 설치

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

## 작업 규칙

### 검증 필수

- **코드 수정 후 반드시 직접 실행해서 동작 확인할 것**
- "완료했습니다"라고 말하기 전에 실제로 테스트해볼 것
- 빌드만 성공했다고 끝이 아님 - 실제 기능이 동작하는지 확인해야 함
- 테스트 불가능한 경우 (터미널 attach 등) 테스트 스크립트를 만들어서라도 검증할 것

### 문서 동기화

- 무언가 작업하고 나면 그 변경사항을 README나 CLAUDE.md 같은 문서에도 반영한다.
