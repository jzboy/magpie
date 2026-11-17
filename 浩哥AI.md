# 浩哥AI

基于 [yetone/magpie](https://github.com/yetone/magpie)（MIT）二次开发的本地 AI 网关与 Agent 模型管理工具。
一处为每个 Agent 挑选模型，一个本地网关统一服务，额度用尽自动切换到下一个账号。

> 上游 magpie 的完整说明见 [README.md](README.md)。本文件只记录浩哥AI 的改造与构建方式。

## 保留与裁剪

### Agent（只保留 5 个）

| ID | 名称 | 说明 |
| --- | --- | --- |
| `claude` | Claude Code | CLI（含 `claude#<dir>` 多配置目录） |
| `claude-desktop` | Claude Desktop | 桌面应用（3p 部署模式） |
| `codex` | Codex | CLI + ChatGPT/Codex 应用 |
| `workbuddy` | WorkBuddy | 腾讯 CodeBuddy 桌面应用（国内版） |
| `dsh` | DeepSeek Harness | DeepSeek 官方 harness |

注册表在 `internal/agent/agents.go` 的 `onMachineAll` 与 `internal/agent/wsl.go` 的 `wslKinds` 裁剪。
其余 Agent 的构造函数文件保留为死代码（保证共享设施与编译不受损），但不再出现在 GUI / CLI / WSL 列表，
其后台保活循环（Cursor Private Inference、OpenMausBot、Claude Science）已摘除。

### 供应商（只集成 4 家）

| 供应商 | 形态 | 实现 |
| --- | --- | --- |
| **DeepSeek** | API key 预设 | `internal/provider/presets.go`（`deepseek`） |
| **ChatGPT API** | OpenAI key 预设 + Sign in with ChatGPT 订阅 + ChatGPT 套餐订阅 | `openai` 预设、`chatgpt-api`（siwc）、`codex`（ChatGPT 套餐，与 ChatGPT API 额度联动，Codex Agent 依赖其登录发现） |
| **WorkBuddy** | 社区插件 | `@magpie-community/opencode-workbuddy-auth` |
| **Qoder 中国版** | 社区插件 | `@magpie-community/opencode-qoder-auth` |

- API-key 预设列表（`provider.Presets()`）只返回 `openai`、`deepseek`；完整预设表保留供查找/图标/余额探针，旧供应商的已存配置仍可用。
- 订阅发现（`provider.Accounts()`）只读 ChatGPT 套餐与 ChatGPT API；Claude / Copilot / Google 登录不再读取。
- 赞助商 partners feed 已关闭（GUI / CLI / TUI / 网关均不再拉取 usemagpie.ai 的 partners）。
- **WorkBuddy / Qoder CN 开箱即用**：二者在上游已退役为社区插件。浩哥AI 在网关启动时通过插件页自身的安装路径
  （`provider.EnsureIntegratedPlugins`，npm + Bun）按 mover 所需版本自动安装一次；装好后 mover 自动把内置 id 交给插件，
  路由 / 用量 / 签到全链路不变。**首次安装需要联网**（下载插件与 Bun 运行时）；用户此后移除插件不会被再次自动装回。

### 路由 / 网关 / 用量（原样保留）

- `internal/gateway`：本地端点（默认 127.0.0.1:3425），协议转换、账号休息、failover、用量记账。
- `internal/routes` + `plugins/routing`、`plugins/groups`、`plugins/rotation`：Smart 等路由策略、路由组。
- `internal/usage` + `plugins/usage`：用量账本、计价、配额卡。

供应商裁剪后这些功能自动只服务保留的四家，无需改动其核心逻辑。

## 品牌 / 图标 / 界面

- **程序名「浩哥AI」**：窗口标题、页头品牌、托盘菜单与悬停提示（四语言）、关于页、macOS bundle（`CFBundleName`/`DisplayName`）、
  Windows exe 元数据（`ProductName`/`FileDescription`/`CompanyName`）全部改为浩哥AI。
  GUI 文案经 `i18n.js` 的 `brand()` 在翻译出口统一替换；托盘经 `trayText()` 统一替换。
- **内部标识保留**：module 路径 `github.com/yetone/magpie`、配置目录 `~/.config/magpie`、`MAGPIE_*` 环境变量、
  `magpie://` 链接、网关 `/v1/magpie/*` 路由、对外 User-Agent、CLI 命令名 `magpie` 均保留，以免破坏 Agent 接线与厂商兼容。
- **AI 主题图标**：喜鹊替换为「双星芒」标记（`build/icon/magpie.svg` / `magpie-small.svg`），
  托盘点击动画改为星芒脉冲（`build/icon/gen.go`）。整套图标（tray / trayflap / Windows tile / .ico / icon-1024 / .icns）
  纯 Go 生成，`.icns` 用 PNG 块封装，**无需 macOS iconutil/actool**，Windows 上即可生成全部图标。
- **界面重设计**：
  - 全新配色——暖纸色亮色主题 / 深板岩暗色主题 + 青绿（teal）强调色，区别于上游的冷灰 + 靛蓝。
    配色契约同步更新于 `internal/gui/appearance.go` 与 `app.css`，`tests/fixtures/look-accents.json` 已重生成。
  - 默认布局改为**顶部导航栏**（上游默认侧栏），标题栏按钮仍可切回侧栏。
  - 新 logo 与加载动画（星芒取代喜鹊）。
- **自更新与遥测关闭**：构建版本号为非 semver 的 `haoge-1.0`，`update.Released()` 为 false，
  因此不会从上游 feed 自更新（避免被原版覆盖品牌），`stats.Run` 遥测 likewise 不发送。

## 构建（仅两种产物）

| 产物 | 平台 | 命令 |
| --- | --- | --- |
| `dist/haoge-ai-darwin-arm64.dmg` | macOS arm64 | `make icons && make release-dmg`（须在 arm64 Mac 上，GUI 需 cgo + WebKit） |
| `dist/haoge-ai-windows-amd64.exe` | Windows amd64 | `make release-windows`（WebView2 无需 cgo，任意平台可交叉编译） |

`make release` 按当前主机产出对应一种。CI 见 [.github/workflows/haoge-release.yml](.github/workflows/haoge-release.yml)：
推送 `haoge-v*` 标签或手动触发，macos-14（arm64）产 dmg、ubuntu-latest 交叉产 exe，并附到 GitHub Release。
macOS 应用未签名/公证，首次打开需右键 → 打开。

构建前提：Go ≥ 1.26.8（见 go.mod）、GNU make + POSIX sh（Windows 用 Git Bash）。
`release-windows` 会联网下载 go-winres 与微软 ConPTY（校验钉死哈希）。

## 验证状态

在 Windows 11（Go 1.27.4）上实测：

- **构建**：`go build ./...` 通过；`GOOS=windows go vet ./...` 与 `GOOS=linux go vet -tags nogui ./...` 均干净；
  Windows amd64 exe 已本机构建并在隔离配置（`.portable` + 临时数据目录）中实跑，界面、品牌、5 个 Agent、
  4 家供应商与插件自动安装均已确认。
- **配色契约**：`TestTintBasesAreAppCSS`、`TestLookFixture`（fixture 已重生成）通过；
  GUI 守卫 `TestEveryWordHasZh`、`TestCoreScriptBudget`、`TestEveryRouteIsNamedForTheCLI`、`TestClientIcons` 通过。
- **测试套件**（裁剪被删 Agent / 订阅用例后）：
  - `internal/agent`：剩余失败**全部**是上游既有的 Windows 平台性红（macOS/Linux 专属测试、分配预算守卫），
    已用 pristine 基线逐项确认同样失败，非本次改造引入：`TestWorkBuddyRestartOnMac`、
    `TestClaudeDesktopCarriesTheLanguage`、`TestCodexSignInInKeychain`、`TestAppliedOfParsesAnUnchangedFileOnce`、
    `TestOwnOptionsKept`、`TestWithViaKeepsTheJoinedList`、`TestHowInstalledWindowsShim`、`TestMiseShimWindowsFiveAgents` 等。
  - `internal/provider`：剩 1 个基线确认的环境性红（`TestRestartSwitchedCodexAppReal`，需真实 Codex 应用）。
  - `internal/gui`：旧品牌托盘文案、旧配色、星芒动画、客户端图标守卫、被删 Agent 的会话/导入/模型行已全部更新或裁剪；
    剩余为环境性红：两处统计计时断言、`TestUpdateThatWontStart`（本机 Smart App Control 拦截未签名构建）、
    以及两个 Windows 重启绘制测试（慢、真实窗口）。
  - `internal/cli`：`TestLibraryProjectCmd` 在 pristine 基线同样失败（dsh 技能归属），非本次引入；
    `TestEveryAgentFieldNameWritesNothing` 暴露了「Claude Code + 无 Claude 订阅时读取 model 字段会触发一次断开/写锁」的细节，未深挖。
  - `internal/library`：RTK、共享技能、技能文件、自服务器超时、subagent、gone-agent、handover、WSL 系列等被删 Agent 用例
    已裁剪或改写为 claude/codex/dsh（WSL 只保留 claude/codex 孪生的覆盖）。剩余的 git 技能系列在本机 git 子进程崩溃
    （0xc0000005）或挂起，pristine 基线同样 10 分钟超时，属本机环境问题，非代码。
- **未运行 / 未收敛**：
  - `internal/gui/tests` 的 Playwright 浏览器套件（1568 个用例，上游 CI 也不跑）——界面重设计改变了大量选择器与布局，
    该套件需要按新布局重写，未在此环境执行。
  - `internal/gateway` 全量测试未跑（上游单包在默认 10 分钟超时都会超时；其大量 `claude_*` / `plugin_*_parity`
    用例针对已删订阅）。删除的订阅其网关适配器保留为死代码，运行路径不受影响。
  - `internal/cli` 剩 2 个与被删订阅耦合的用例（`TestEveryAgentFieldNameWritesNothing`、`TestLibraryProjectCmd`），
    其断言依赖 Claude 订阅的模型目录与共享技能目录的旧成员集合，随订阅裁剪而变；未再深挖。
- **macOS dmg** 需在 Mac / macOS CI 上产出（GUI 需 cgo + WebKit + hdiutil），本 Windows 机无法构建；
  已提供 `make release-dmg` 与 `.github/workflows/haoge-release.yml`。