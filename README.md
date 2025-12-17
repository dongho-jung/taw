# Workbench

Claude Code 기반의 프로젝트 관리 시스템입니다.

## 개요

- 프로젝트별로 zellij 세션 기반의 작업 환경을 제공합니다.
- 태스크를 생성하면 자동으로 Claude Code agent가 시작됩니다.

## 디렉토리 구조

```
workbench/
├── add-project              # 새 프로젝트 추가
├── _workbench/              # 전역 설정
│   ├── PROMPT.md            # 전역 에이전트 프롬프트
│   ├── start                # 세션 시작 스크립트
│   ├── bin/                 # 공용 실행 파일 (PATH에 추가됨)
│   │   └── new-task         # 태스크 생성
│   └── layout.kdl           # zellij 레이아웃
└── projects/{프로젝트명}/
    ├── start                # -> _workbench/start
    ├── location             # -> 실제 프로젝트 경로
    ├── PROMPT.md            # 프로젝트별 프롬프트
    └── agents/{태스크명}/
        ├── task             # 태스크 내용
        ├── log              # 진행 로그
        └── worktree/        # git worktree
```

## 사용법

### 프로젝트 추가

```bash
./add-project  # yazi에서 git 프로젝트 선택 → 자동으로 세션 시작
```

### 프로젝트 시작

```bash
cd projects/{프로젝트명} && ./start  # 기존 세션이 있으면 attach
```

### 태스크 생성

```bash
new-task  # $EDITOR에서 태스크 작성 → 자동으로 agent 시작
```

### 탭 상태

- 🤖 작업 중
- 💬 대기 중
- ✅ 완료

## 설정

- `_workbench/PROMPT.md`: 전역 에이전트 프롬프트
- `{프로젝트}/PROMPT.md`: 프로젝트별 프롬프트
- `EDITOR` 환경변수: 태스크 작성 에디터 (기본: vim)

## 의존성

```bash
brew install zellij fswatch yazi
```

## zellij 단축키

- `Ctrl+O, d`: detach
- `Ctrl+O, w`: 탭 전환
