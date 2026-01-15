# PAW + claude-mem 메모리 통합 안내

이 문서는 PAW에서 기존 `.paw/memory` 기반 메모리를 제거하고, `claude-mem` 플러그인을 통해
여러 workspace/agent 간 메모리를 공유하도록 변경된 내용을 설명합니다.

## 핵심 요약

- **기존 `.paw/memory`는 더 이상 사용하지 않습니다.**
- **메모리는 claude-mem 플러그인이 자동으로 저장/검색합니다.**
- **PAW는 작업 디렉토리를 일관되게 맞춰** 프로젝트 단위 메모리 공유가 되도록 했습니다.

## 무엇이 어떻게 추가/변경되었나

### 1) 메모리 저장 주체 변경

- 과거: PAW가 `.paw/memory` 파일을 만들고 업데이트하도록 설계됨.
- 현재: **claude-mem이 모든 메모리를 자동으로 저장**합니다.
- `.paw/memory` 생성/사용 로직은 제거되었습니다.

### 2) 프로젝트 이름(메모리 키) 일관성 확보

claude-mem은 **현재 작업 디렉토리의 이름(basename)** 을 프로젝트 키로 사용합니다.
작업 디렉토리가 매번 달라지면 메모리가 분리되기 때문에, PAW가 이를 고정했습니다.

- **Git 작업(워크트리 모드)**
  - 각 task의 워크트리 디렉토리를 `{project-name}`로 생성합니다.
  - 예: `.paw/agents/{task}/my-repo/`
  - 이렇게 하면 모든 task가 동일한 프로젝트 키(`my-repo`)로 메모리를 공유합니다.
  - 기존에 `worktree/`로 만들어진 작업은 그대로 유지하며, 새 작업부터 `{project-name}`로 생성합니다.

- **Non-Git 작업**
  - Claude의 작업 디렉토리를 **프로젝트 루트(`$PROJECT_DIR`)**로 고정했습니다.
  - 따라서 언제 시작해도 프로젝트 키가 동일합니다.

### 3) plugin 체크 추가

- `paw check`에서 **claude-mem 플러그인 설치 여부**를 확인합니다.

## claude-mem 동작 방식(간단 설명)

claude-mem은 Claude Code의 훅을 사용해 자동으로 메모리를 축적합니다.

- **SessionStart**: 기존 메모리를 주입
- **UserPromptSubmit**: 세션 초기화
- **PostToolUse**: tool 사용 기록을 관찰(Observation)로 저장
- **Stop**: 세션 요약 저장

이 모든 과정은 Claude Code 플러그인으로 자동 처리됩니다.

## 설치 방법

Claude Code 안에서 아래 명령을 실행합니다.

```
> /plugin marketplace add thedotmack/claude-mem
> /plugin install claude-mem
```

설치 후 **Claude Code 재시작**이 필요합니다.

## 사용 방법

### 자동 저장

- 별도 조작 없이 **모든 세션/툴 사용 기록이 자동 저장**됩니다.
- 민감한 정보는 `<private>...</private>`로 감싸면 저장되지 않습니다.

### 검색/조회

1) **자연어 질의**
- "이 프로젝트에서 지난주에 했던 변경 사항 알려줘" 등으로 질문하면 자동 활용됩니다.

2) **MCP 도구 사용** (효율적 검색)
- `search` → `timeline` → `get_observations` 3단계로 필요한 정보만 불러옵니다.

### 웹 뷰어

- `http://localhost:37777`에서 메모리 스트림과 설정을 확인할 수 있습니다.

## 데이터 위치

- 메모리 데이터: `~/.claude-mem/`
- 설정 파일: `~/.claude-mem/settings.json`

## 주의사항

- 프로젝트 키는 **작업 디렉토리 이름**입니다.
  - 동일한 이름의 다른 프로젝트가 있으면 메모리가 섞일 수 있습니다.
  - 필요하면 프로젝트 디렉토리 이름을 구분해 주세요.
- `.paw/memory`는 더 이상 사용하지 않습니다.

## 변경 사항 요약

- `.paw/memory` 제거
- 워크트리 경로를 `{project-name}`으로 변경 (신규 작업부터 적용)
- Non-Git 작업은 `$PROJECT_DIR`에서 실행
- `paw check`에 claude-mem 플러그인 점검 추가
- 문서 및 프롬프트 업데이트
