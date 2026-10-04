# Zen Gate — 本地免费模型网关

一个约 8MB 的 Windows 托盘程序：把 **OpenCode Zen 免费车道**变成你本机的
OpenAI / Anthropic 兼容 API，并**自动配置本机已装的 AI Agent**——打开开关，
对应 Agent 的模型选择器里就会出现这些免费模型。

```
D:\opencode zen\zen-gate\dist\zen-gate.exe
```

## 快速开始

1. 双击 `dist\zen-gate.exe`（托盘图标出现，管理页自动在浏览器打开）
2. 在「Agent 自动适配」区打开你想接的 Agent 开关
3. 重启对应 Agent → 模型选择器出现 `Zen Gate` 分组下的免费模型
4. 托盘右键：打开管理页 / 复制接入地址 / 退出

## Agent 适配

| Agent | 注入点 | 状态 |
| --- | --- | --- |
| ZCode | `~/.zcode/v2/provider_config.json`（openai-chat-completions 渠道） | ✅ 实测 |
| OpenCode | `~/.config/opencode/opencode.json`（@ai-sdk/openai-compatible） | ✅ 实测 |
| Codex CLI/桌面版 | `~/.codex/config.toml`（`[model_providers.zen_gate]`, wire_api=chat） | ✅ 实测 |
| Claude Code | `~/.claude/settings.json` env（ANTHROPIC_BASE_URL 等，原值自动备份还原） | ✅ 实测 |
| DeepSeek Harness | 安装/升级 `dsh-our-free-model` 插件到 profiles | ✅ 实测 |
| Crush | `~/.config/crush/crush.json` providers（openai-compat，含模型元数据） | ✅ 模拟实测 |
| ChatBox | `%APPDATA%\ChatBox\chatbox.config.json` openai 渠道 | ✅ 模拟实测 |
| Aider | `~/.aider.conf.yml` openai-api-base/key（marker 块） | ✅ |
| Qwen Code | `~/.qwen/settings.json` modelProviders + `.env` 凭据 | ✅ |
| Continue (VS Code) | `~/.continue/config.yaml` models 块（已有 models 时转手动） | ✅ |

开启任一 Agent 会生成一个独立子 Key（便于分开统计用量）。
所有写入前都会备份原文件到 `%APPDATA%\zen-gate\backups\<agent>\`；关闭开关即还原。
目标应用正在运行时会提示「需重启生效」。

无法自动注入的客户端（Cherry Studio 未公开配置格式、Cline/Roo 的 SQLite 状态、ChatGPT 官方版、Gemini CLI 等）
在管理页手动复制 Base URL + Key 接入即可。

## API

```
GET  /v1/models                OpenAI 模型列表
POST /v1/chat/completions      流式/非流式/工具调用/图片（reasoning 走 delta.reasoning）
POST /v1/responses             OpenAI Responses 协议（Codex 系）
POST /v1/messages              Anthropic 协议（Claude Code 系）
```

模型名可带思考档位后缀：`mimo-v2.6-flash-free (deep)` / `(light)` / `(balanced)`。
档位是**强制下发的输出 token 预算**（2048 / 8192 / 模型上限；思考关不掉的模型翻倍），
不是被上游忽略的 reasoning_effort 字符串。

```bash
curl http://127.0.0.1:8787/v1/chat/completions \
  -H "Authorization: Bearer ofm-…" -H "content-type: application/json" \
  -d '{"model":"mimo-v2.6-flash-free (deep)","messages":[{"role":"user","content":"你好"}],"stream":true}'
```

## 工程行为（移植自 dsh-our-free-model v1.3.2）

- **会话稳定映射**：同一对话永远映射同一上游 session（免费额度按会话计，乱铸 id 会 429）
- **工具四件套门**：自动补齐 `bash/glob/grep/read` 指纹声明（真实工具优先晋升，缺槽补自禁用诱饵）
- **按 body 形状嗅探流式**：网关高负载时用 JSON content-type 回 SSE 也不会解码失败
- **截断分类**：上游中途掐流不再被当成正常结束；纯思考掐断自动用检查点续写一次（≤480s 总限）
- **429 退避**：不硬撞重试；探测全 429 后 30→120 分钟指数退避
- **可用性探测**：每 15 分钟实测（只有网关点名拒绝才从列表摘模型，列表永不为空）
- 管理台只绑 127.0.0.1；主 Key `timingSafeEqual` 比对；配置原子写入

## 已知边界

- 免费车道按会话限速，多 Agent 并发打满会 429（管理页显示「已限额」）
- 上游随时可能收紧免费额度或地区门（管理页按出口 IP 如实显示 region 状态）
- 本工具仅供本机个人使用；使用免费额度受上游提供方条款约束

## 归属

协议层行为移植自 MIT 项目 [dsh-our-free-model](https://github.com/zouyuxuan122/dsh-our-free-model)
(v1.3.2, © zouyuxuan122)，内嵌的 DeepSeek Harness 插件即为该项目的发布文件（MIT）。

## 开发

```
go test ./...                          # 单元测试（假上游，不出网）
go build -o dist/zen-gate.exe ./cmd/zen-gate                    # 控制台调试版
go build -trimpath -ldflags "-s -w -H=windowsgui" -o dist/zen-gate.exe ./cmd/zen-gate   # 托盘正式版
zen-gate.exe --no-tray --port 8787     # 控制台模式运行
```

数据目录：`%APPDATA%\zen-gate\`（config.json / stats.json / backups\）
