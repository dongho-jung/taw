# PAW (Parallel AI Workers)

[![CI](https://github.com/dongho-jung/paw/actions/workflows/ci.yml/badge.svg)](https://github.com/dongho-jung/paw/actions/workflows/ci.yml)
[![Release](https://github.com/dongho-jung/paw/actions/workflows/release.yml/badge.svg)](https://github.com/dongho-jung/paw/actions/workflows/release.yml)

생각나는대로 에이전트에게 이것저것 시키고 싶은데, 두가지 어려움이 있었습니다:
  1. 동시에 이것저것 시키다보면, **서로 작업하다가 충돌이 나곤 한다**. -> "충돌을 해결하느라 더 많은 시간과 공수를 쓰게 된다."
  2. 동시에 이것저것 시키다보면, **각각의 진행상황을 파악하기가 어렵다**. -> "개발 사이클이 늘어지고 몰입이 깨진다."

PAW는 이런 어려움을 다음과 같이 해결합니다:
  1. "동시에 이것저것 시키다보면, 서로 작업하다가 충돌이 나곤 한다." -> Git Worktree를 활용해, **에이전트마다 격리된 작업공간을 주어 해결합니다**.
  2. "동시에 이것저것 시키다보면, 각각의 진행상황을 파악하기가 어렵다." -> Kanban UI를 활용해, **작업별 진행상황을 쉽게 추적할 수 있게 하여 해결합니다**.

비슷한 다른 접근 대비, PAW가 가지는 강점은 다음과 같습니다:
  - 완전히 터미널 내에서만 동작하므로,
    - **가볍고 빠릅니다**.
    - 따로 포트를 열고 브라우저로 접속하거나 Electron 앱 같은걸 따로 설치하고 열어둘 필요가 없습니다.
    - SSH 접속만 되면 **밖에서도 언제든지 일을 시킬 수 있습니다**.
  - tmux를 활용하여,
    - **네트워크 연결이 불안정한 곳에서도 사용하기 용이합니다**.
    - 하나의 작업을 노트북, 데스크탑, 폰 **어디서든 이어서 작업할 수 있습니다**.
  - Git Workflow를 자동화 하여,
    - **브랜치 이름 같은건 고민하지 않아도 됩니다**. 작업 내용을 보고 에이전트가 알아서 지어줍니다.
    - **머지할때 충돌도 고민하지 않아도 됩니다**. 충돌 나면 에이전트가 알아서 풀어줍니다.
    - **PR 날릴때도 고민하지 않아도 됩니다**. PR 내용 작성부터 제출까지 에이전트가 알아서 해줍니다.
    - **외부에서 PR이 머지될때도 로컬의 브랜치나 Worktree는 신경쓰지 않아도 됩니다**. 에이전트가 알아서 정리해줍니다.
  - OSC를 활용하여,
    - 단순 터미널 알람이 아니라, **좀 더 맥락을 담은 알람을 보내줍니다**.
    - 터미널 색상을 자동으로 인지하고, **가독성 좋은 테마를 적용합니다**.

## 설치 & 제거

### brew
```bash
brew install dongho-jung/tap/paw
```
```bash
brew uninstall dongho-jung/tap/paw
```

### go
```bash
go install github.com/dongho-jung/paw/cmd/paw@latest
```
```bash
rm $(go env GOPATH)/bin/paw
```

### local
```bash
git clone https://github.com/dongho-jung/paw
cd paw
make build && make install
```
```bash
make uninstall
```
