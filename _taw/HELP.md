# TAW (Tmux + Agent + Worktree)

Claude Code 기반 자율 에이전트 작업 환경

## 키보드 단축키

### 태스크 관리
  ^a          팝업 쉘 열기/닫기 (현재 worktree 경로)
  ^n          새 태스크 생성 (에디터 열림)
  ^x          현재 태스크 종료 (worktree/branch 정리)
  ^⌥a         빠른 태스크 큐 추가 (완료 후 자동 처리)

### Pane 조작
  ⌥ ←/→/↑/↓   pane 이동
  ⌥ h         왼쪽에 새 pane 생성
  ⌥ j         아래에 새 pane 생성
  ⌥ k         위에 새 pane 생성
  ⌥ l         오른쪽에 새 pane 생성
  ⌥ x         현재 pane 닫기

### Window 조작
  ⇧⌥ ←/→      이전/다음 window로 이동

### 세션
  ^q          세션에서 나가기 (detach)
  ⌥ /         이 도움말 보기

## Slash Commands (에이전트용)

  /commit     스마트 커밋 (diff 분석 후 메시지 자동 생성)
  /test       프로젝트 테스트 자동 감지 및 실행
  /pr         PR 자동 생성 및 브라우저 열기
  /merge      worktree 브랜치를 프로젝트 브랜치에 머지
  /finish     태스크 완료 (commit → PR → 상태 업데이트)
  /done       태스크 정리 (worktree, branch, window 삭제)

## 디렉토리 구조

  .taw/
  ├── PROMPT.md              프로젝트별 에이전트 지시사항
  ├── new-task               태스크 생성 스크립트
  ├── .queue/                빠른 태스크 큐 (^⌥a로 추가)
  └── agents/{task-name}/
      ├── task               태스크 내용
      ├── log                진행 로그
      ├── attach             태스크 재연결 (./attach 실행)
      ├── origin/            프로젝트 루트 (symlink)
      └── worktree/          git worktree (자동 생성)

## Window 상태 아이콘

  🤖  에이전트 작업 중
  💬  사용자 입력 대기
  ✅  작업 완료

## 환경변수 (에이전트용)

  TASK_NAME     태스크 이름
  TAW_DIR       .taw 디렉토리 경로
  PROJECT_DIR   프로젝트 루트 경로
  WORKTREE_DIR  worktree 경로
  WINDOW_ID     tmux window ID (상태 갱신용)

---
q 키로 나가기
