# Plan: Improve Task Window Flow

## Problem Statement

현재 `new-task` 실행 시:
1. `⭐️new` 윈도우에서 task 내용 입력 후
2. "Generating task name..." 스피너가 **같은 윈도우**에서 돌아감
3. 스피너가 완료될 때까지 새 task 생성 불가

## Desired Behavior

1. `⭐️new` 윈도우에서 task 내용 입력 후
2. **즉시** 다음 task 입력 가능하도록 `⭐️new` 윈도우 유지
3. "Generating..." 및 task setup은 **별도 윈도우**에서 진행

## Implementation Steps

### Step 1: Modify `newTaskCmd` in `cmd/taw/internal.go`

현재 flow (lines 100-187):
```
1. Open editor → get content
2. Run spinner IN SAME WINDOW (blocks)
3. Call mgr.CreateTask(content)
4. Spawn handle-task
5. Wait for window_id
6. Loop to next task
```

새로운 flow:
```
1. Open editor → get content
2. Spawn background process for task creation (NO spinner in new window)
3. IMMEDIATELY loop to next task
```

주요 변경:
- `newTaskCmd`에서 spinner 제거
- task content를 파라미터로 받는 새 internal command `spawn-task` 생성
- `spawn-task`가 별도 윈도우 생성 후 spinner 표시

### Step 2: Create new `spawn-task` command

새로운 command `taw internal spawn-task <session> <content-file>`:
1. 임시 "⏳..." 윈도우 생성
2. 해당 윈도우에서 spinner 표시
3. Task 생성 (Claude API 호출)
4. `handle-task` 로직 실행
5. 완료 후 임시 윈도우 닫고 실제 task 윈도우로 전환

### Step 3: Task Content Passing

Task content를 전달하는 방법:
- 임시 파일에 content 저장
- `spawn-task`에 파일 경로 전달
- `spawn-task`가 파일 읽고 삭제

## Files to Modify

1. `cmd/taw/internal.go`:
   - `newTaskCmd`: spinner 제거, spawn-task 호출
   - 새로운 `spawnTaskCmd` 추가

2. `internal/constants/constants.go`:
   - 임시 윈도우 이름 상수 추가 (optional)

## Validation

✅ **자동 검증 가능**:
- `make build` 성공
- 코드 분석으로 flow 확인:
  1. `newTaskCmd`가 spinner 없이 바로 반환하는지
  2. `spawn-task`가 별도 윈도우에서 실행되는지

⚠️ **수동 테스트 권장**:
- 실제 tmux 환경에서 연속 task 생성 테스트
- 윈도우 전환 동작 확인

## Risk Assessment

- Low risk: 기존 로직을 크게 바꾸지 않고 spawn 방식만 변경
- Backward compatible: 기존 task 구조 유지
