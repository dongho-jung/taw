# PAW (Parallel AI Workers)

[![CI](https://github.com/dongho-jung/paw/actions/workflows/ci.yml/badge.svg)](https://github.com/dongho-jung/paw/actions/workflows/ci.yml)
[![Release](https://github.com/dongho-jung/paw/actions/workflows/release.yml/badge.svg)](https://github.com/dongho-jung/paw/actions/workflows/release.yml)

![](./.github/assets/demo_1.png)
![](./.github/assets/demo_2.png)
![](./.github/assets/demo_3.png)

생각나는대로 에이전트에게 이것저것 시키고 싶은데, 두가지 어려움이 있었습니다:
  1. 동시에 이것저것 시키다보면, **서로 작업하다가 충돌이 나곤 한다**. -> "충돌을 해결하느라 더 많은 시간과 공수를 쓰게 된다."
  2. 동시에 이것저것 시키다보면, **각각의 진행상황을 파악하기가 어렵다**. -> "개발 사이클이 늘어지고 몰입이 깨진다."

PAW는 이런 어려움을 다음과 같이 해결합니다:
  1. "동시에 이것저것 시키다보면, 서로 작업하다가 충돌이 나곤 한다." -> Git Worktree를 활용해, **에이전트마다 격리된 작업공간을 주어 해결합니다**.
  2. "동시에 이것저것 시키다보면, 각각의 진행상황을 파악하기가 어렵다." -> Kanban UI를 활용해, **작업별 진행상황을 쉽게 추적할 수 있게 하여 해결합니다**.

PAW가 제공하는 것들:
  - Git Workflow가 자동화 되어있어, 작업을 시키는 것에만 집중할 수 있습니다.
  - 완전히 터미널 내에서만 동작하므로, 가볍고 빠릅니다.
  - PAW가 PAW에 대해 알고 있으므로, 동작방식을 바꾸고 싶을땐 그냥 작업을 시키듯이 요청하면 됩니다.
    - e.g. 새 task 만들때 worktree에 .terraform 복사해줘, 커밋할땐 @~~ 규칙을 준수해줘, ...
  - 마우스 상호작용을 지원합니다.
  - tmux로 세션을 관리하므로, 네트워크 연결이 불안정한 곳이나 서버에서 사용하기 용이합니다.
 

# 설치 & 제거

## brew
```bash
brew install dongho-jung/tap/paw
```
```bash
brew uninstall dongho-jung/tap/paw
```

## go
```bash
go install github.com/dongho-jung/paw/cmd/paw@latest
```
```bash
rm $(go env GOPATH)/bin/paw
```

## manual
```bash
git clone https://github.com/dongho-jung/paw
cd paw
make build && make install
```
```bash
make uninstall
```

# 사용법

## 기본 흐름
1. 작업할 디렉토리로 이동
2. `paw`로 시작
3. 시킬 작업을 작성, `alt + enter`로 제출
4. 자동으로 적절한 branch와 worktree가 만들어지고 작업 시작
5. 작업이 완료되면, 해당 task window에서 `ctrl + f`로 마무리

**다른 작업들이 진행중이어도 서로 격리된 공간에 있기 때문에 충돌이 잘 나지 않습니다.**  
**충돌이 나더라도 `ctrl + f`로 마무리할때 Claude가 충돌을 해결 합니다.**

## 기본 조작
- window(task)간 이동: `alt + left/right key`
- pane간 cycle: `alt + tab`
- 작업 마무리(Finish): `ctrl + f`
- paw 나가기(Quit): `ctrl + q` 

## 추가 조작
- `ctrl + r`: 이전에 작성한 task 불러오기
- `ctrl + t`: 템플릿 불러오기
- `ctrl + g`: Git Viewer
- `ctrl + o`: Log Viewer
- `ctrl + p`: Command Pallete
- `ctrl + j`: Switch Projects
