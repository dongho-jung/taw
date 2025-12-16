# Workbench

Claude Code 기반의 자동화된 태스크 관리 시스템입니다.

## 개요

- 프로젝트별로 태스크를 관리하고, 파일 변경을 감지하여 Claude Code가 자동으로 처리합니다.
- zellij 세션 기반으로 동작하며, 태스크별로 별도 탭이 생성됩니다.

## 디렉토리 구조

```
workbench/
├── add-project              # 새 프로젝트 추가 스크립트
├── README.md
├── _workbench/              # 전역 설정
│   ├── PROMPT.md            # 전역 시스템 프롬프트
│   ├── start                # start 스크립트 템플릿
│   └── layout.kdl           # zellij 레이아웃
└── projects/
    └── {프로젝트명}/
        ├── start            # 프로젝트 시작 스크립트
        ├── PROMPT.md        # 프로젝트별 프롬프트
        ├── metadata         # 프로젝트 메타데이터
        ├── location         # 실제 프로젝트 경로 (심볼릭 링크)
        ├── .watcher/        # watcher 상태
        │   ├── watcher.log
        │   └── watcher.pid
        ├── agents/          # 에이전트 작업 공간
        │   └── {태스크명}/
        │       ├── log      # 에이전트 로그
        │       ├── task.json # 태스크 정보
        │       └── worktree # git worktree (작업용)
        ├── to-do/           # 대기 중인 태스크
        ├── in-progress/     # 진행 중인 태스크
        ├── in-review/       # 리뷰 대기 중
        ├── done/            # 완료된 태스크
        └── cancelled/       # 취소된 태스크
```

## 사용법

### 1. 프로젝트 추가

```bash
./add-project
```

- yazi가 열리면 관리할 프로젝트 디렉토리 선택
- 자동으로 `projects/{프로젝트명}/` 생성 후 해당 경로로 이동

### 2. 프로젝트 시작

```bash
./start          # zellij 세션 시작 + 파일 감시
./start stop     # 세션 종료
./start status   # 상태 확인
./start log      # watcher 로그 확인
```

- 세션이 이미 실행 중이면 자동으로 attach
- zellij 종료 시 fswatch도 자동 정리

### 3. 태스크 생성

```bash
echo "버그 수정해줘" > to-do/fix-bug.md
```

파일이 생성되면:
1. 새 zellij 탭 생성: `fix-bug/to-do`
2. 탭에서 수직 분할 (왼쪽: claude, 오른쪽: shell)
3. Claude Code 실행 및 태스크 전달
4. `agents/fix-bug/` 디렉토리 생성
5. 태스크 정보가 `agents/fix-bug/task.json`에 저장

### 4. 태스크 상태 변경

파일을 다른 디렉토리로 이동하면 상태가 변경됩니다:

```bash
mv to-do/fix-bug.md in-progress/    # 진행 중으로 변경
mv in-progress/fix-bug.md done/     # 완료로 변경
mv in-progress/fix-bug.md cancelled/ # 취소
```

## 설정

### _workbench/PROMPT.md

전역 시스템 프롬프트입니다. 모든 프로젝트의 Claude Code에 적용됩니다.

주요 내용:
- 태스크 처리 규칙
- 디렉토리 구조 설명
- 상태별 동작 정의
- git worktree 사용법
- 로깅 규칙

### {프로젝트}/PROMPT.md

프로젝트별 프롬프트입니다. 해당 프로젝트에만 적용됩니다.

### _workbench/layout.kdl

zellij 레이아웃 설정:
- 상단: 탭 바
- 하단: 상태 바
- 기본 탭 이름: `_`

## 의존성

```bash
brew install zellij    # 터미널 멀티플렉서
brew install fswatch   # 파일 변경 감시
brew install yazi      # 파일 매니저
```

## 권한

프로젝트 루트 디렉토리는 쓰기 금지됩니다 (`chmod a-w`).
파일은 하위 디렉토리에만 생성할 수 있습니다:
- `to-do/`, `in-progress/`, `in-review/`, `done/`, `cancelled/`, `agents/`

## zellij 단축키

- `Ctrl+O, d`: 세션에서 분리 (detach)
- `Ctrl+O, w`: 탭 전환
- `Ctrl+O, n`: 새 pane
- `Ctrl+O, x`: pane 닫기
