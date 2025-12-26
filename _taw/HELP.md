# TAW (Tmux + Agent + Worktree)

Claude Code 기반 자율 에이전트 작업 환경

## 키보드 단축키

### 네비게이션
  ⌥ Tab       다음 pane으로 이동 (순환)
  ⌥ ←/→       이전/다음 window로 이동

### 태스크 관리
  ⌥ n         새 태스크 생성 (에디터 열림)
  ⌥ e         태스크 완료 (commit → PR/merge → cleanup, ON_COMPLETE 설정 따름)
  ⌥ m         완료된 태스크 일괄 머지 (✅ 상태 태스크 모두 merge + end)
  ⌥ p         팝업 쉘 열기/닫기 (현재 worktree 경로)
  ⌥ u         빠른 태스크 큐 추가 (완료 후 자동 처리)

### 세션
  ⌥ q         세션에서 나가기 (detach)
  ⌥ /         이 도움말 열기/닫기 (toggle)

## Slash Commands (에이전트용)

  /commit     스마트 커밋 (diff 분석 후 메시지 자동 생성)
  /test       프로젝트 테스트 자동 감지 및 실행
  /pr         PR 자동 생성 및 브라우저 열기
  /merge      worktree 브랜치를 프로젝트 브랜치에 머지

## 디렉토리 구조

  .taw/
  ├── PROMPT.md              프로젝트별 에이전트 지시사항
  ├── new-task               태스크 생성 스크립트
  ├── .queue/                빠른 태스크 큐 (⌥u로 추가)
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
  ⚠️  손상됨 (복구 또는 정리 필요)

## 환경변수 (에이전트용)

  TASK_NAME     태스크 이름
  TAW_DIR       .taw 디렉토리 경로
  PROJECT_DIR   프로젝트 루트 경로
  WORKTREE_DIR  worktree 경로
  WINDOW_ID     tmux window ID (상태 갱신용)

---
⌥/ 또는 q 키로 나가기
