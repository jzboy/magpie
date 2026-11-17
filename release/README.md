# 浩哥AI 发布产物

本目录存放浩哥AI 的两份构建产物（本分支 `haoge-ai`）。

| 文件 | 平台 | 说明 |
| --- | --- | --- |
| `haoge-ai-windows-amd64.exe` | Windows amd64 | GUI 版（WebView2），已在 Windows 11 上实跑验证 |
| `haoge-ai-windows-amd64.exe.sha256` | — | 上述 exe 的 SHA-256 |
| `haoge-ai-darwin-arm64.dmg` | macOS arm64 | **由 CI 产出**（见下）；Windows 上无法构建 |

## macOS dmg

macOS GUI 版链接 cgo + WebKit，只能在 macOS（arm64）上构建，本机为 Windows，无法产出。
两种方式得到 dmg：

1. **CI（推荐）**：推送 `haoge-v*` 标签，GitHub Actions
   [`.github/workflows/haoge-release.yml`](../.github/workflows/haoge-release.yml) 会在 `macos-14`（arm64）
   跑 `make icons && make release-dmg`，并把 dmg 与 exe 一起附到 GitHub Release：

   ```bash
   git tag haoge-v1.0
   git push jzboy haoge-v1.0
   ```

2. **在 Mac 上本地构建**：

   ```sh
   make icons
   make release-dmg
   # 产物：dist/haoge-ai-darwin-arm64.dmg
   ```

   未签名/未公证，首次打开需右键 → 打开。

## Windows exe

本目录的 exe 由以下命令在 Windows 上构建（Git Bash，需 Go ≥ 1.26.8）：

```bash
make release-windows   # 产物：dist/haoge-ai-windows-amd64.exe
```

或不用 make（本机无 make 时）：

```bash
go run ./build/assetpack internal/gui/assets internal/gui/assets.zip
go run ./build/conpty
go run github.com/tc-hib/go-winres@v0.3.3 make --in build/windows/winres.json --arch amd64 --out rsrc \
  --file-version 0.0.0 --product-version 0.0.0
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags="-s -w" \
  -o internal/update/launcher-amd64.com ./internal/update/launcher
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build \
  -tags production,comlauncher,packedassets,conptypair -trimpath \
  -ldflags="-s -w -X main.version=haoge-1.0 -H windowsgui" \
  -o dist/haoge-ai-windows-amd64.exe .
```

版本号 `haoge-1.0` 是非 semver，`update.Released()` 为 false：应用不会从上游自更新、也不发送遥测。
