# `askgpt` — 一款用于与 GPT 风格 API 交互的命令行工具

`askgpt` 是一个轻量级、注重隐私的命令行界面工具，可让你直接从终端与 GPT 兼容的 API（例如 OpenAI、Azure OpenAI 或自定义端点）进行交互。它开箱即用地支持翻译、摘要、解释等常见任务，同时也允许自由格式的提示和多轮对话。

该工具使用 Go 语言编写，强调简洁性、可配置性以及流式响应，以提供流畅的用户体验。

---

## ✨ 功能特性

- **任务快捷方式**：  
  使用内置提示处理常见的 NLP 任务：
  - `translate-en` — 翻译为英文
  - `translate-zh` — 翻译为中文
  - `summarize` — 摘要内容
  - `explain` — 解释技术性或复杂文本

- **流式响应**：实时逐词（token）显示输出。

- **多行输入与粘贴模式**：  
  - 在行尾使用反斜杠 `\` 可续行输入
  - 输入 `:paste` 可粘贴大段内容（以单独一行 `:end` 结束）

- **持久化配置**：  
  配置安全地存储在 `~/.askgpt/config.yaml`，权限设为 600。

- **灵活的配置格式**：  
  支持映射（mapping）和映射列表（list-of-maps）两种 YAML 样式，提升用户友好性。

- **自说明的 CLI**：  
  包含 `show-config`、`set-url`、`set-model` 和 `set-key` 子命令。

---

## 🚀 安装

### 从源码安装（需 Go 1.21+）

```sh
go install github.com/abnerhexu/askgpt@latest
```

生成的二进制文件将位于 `$GOPATH/bin/askgpt`。

---

## ⚙️ 配置

首次运行时，`askgpt` 会在以下位置创建配置文件：

```
~/.askgpt/config.yaml
```

配置示例（自动生成）：

```yaml
# askgpt config
# 您可直接编辑此文件，或使用：askgpt set-url | set-model | set-key
askgpt:
  - url: https://api.openai.com/v1/chat/completions
  - model: gpt-4o-mini
  - key: sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

> 🔒 该文件以 `0600` 权限创建，以保护您的 API 密钥。

### 通过 CLI 设置配置

```sh
askgpt set-url https://api.openai.com/v1/chat/completions
askgpt set-model gpt-4o-mini
askgpt set-key sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

---

## 🧪 使用方法

### 基本任务

```sh
askgpt summarize
> The quick brown fox jumps over the lazy dog...
```

### 自由格式提示

将任务名视为初始提示：

```sh
askgpt "Explain quantum entanglement in simple terms"
> [按 Enter 或使用 :paste 输入多行内容]
```

### 查看当前配置

```sh
askgpt show-config
```

### 多轮对话

在首次响应后，您可以继续聊天：
- 输入下一条消息
- 输入 `quit` 退出
- 空行将被忽略

---

## 📝 输入提示

- **单行输入**：输入后按 `Enter`。
- **多行输入**：在行尾加 `\` 以继续下一行。
- **粘贴模式**：输入 `:paste`，粘贴内容，然后在单独一行输入 `:end`。
- **退出**：在任意提示符下输入 `quit`。

---

## 🔐 安全说明

- 您的 API 密钥**仅**存储于 `~/.askgpt/config.yaml`，并设置为严格权限（`rw-------`）。
- 本工具不会记录或向配置的 API 端点之外传输您的提示内容。
- 请始终使用 HTTPS 端点。

---

## 🛠 开发

从源码构建：

```sh
git clone https://github.com/abnerhexu/askgpt.git
cd askgpt
go build -o askgpt .
```

---

## 📜 许可证

GPLv3 许可证 — 详情请参见 [LICENSE](LICENSE)。

---

## 🙌 致谢

- 使用 [`gopkg.in/yaml.v3`](https://pkg.go.dev/gopkg.in/yaml.v3) 实现灵活的 YAML 解析。
- 受 `curl`、`httpie` 和 `chatgpt-cli` 等工具启发。
- 本文档由 Qwen3-Max 生成。

---

> 💡 有建议或发现 Bug？欢迎提交 Issue 或 Pull Request！