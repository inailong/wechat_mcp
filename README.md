# wechat_mcp

A lightweight, standalone Model Context Protocol (MCP) server for Linux that enables AI assistants (Claude Desktop, Claude Code, Cursor, Windsurf) to directly query and analyze local WeChat chat history, contacts, and received documents (PDF, Word, Excel, etc.) **without running WeFlow, Electron, or any heavy GUI applications**.

---

## 1. 核心架构与特性 (Architecture & Features)

- **完全独立原生驱动 (Zero WeFlow/Electron Dependency)**:
  - 内置原生 CGO 数据库驱动，直接打开并解密本地微信 SQLite 数据库（`session.db`、`message_0.db`、`contact.db`）。
  - 二进制内部集成动态符号导出（`-Wl,--export-dynamic`），自动实现时钟平滑，**无需配置 `LD_PRELOAD` 或外部包装脚本**。
  - 运行时内存仅占用 **~15MB**，冷启动低于 50ms。

- **多类型文件完整支持 (Full File & Document Support)**:
  - 聊天记录中的所有传输文件（PDF、DOCX、XLSX、PPTX、音视频、压缩包等）在本地均为原生未加密文件。
  - 提供 `list_files` 与 `get_message_file` 工具，AI 可直接定位文件物理路径并读取内容。
  - 在 `list_messages` 中自动为文件类消息（Type 49）附加本地真实文件绝对路径 `filePath`。

- **零配置与自动账户探测 (Zero-Confirmation Discovery)**:
  - 自动定位微信主目录（`~/Documents/xwechat_files/`），自动解密 `global_config` 获取 `wxid` 与昵称。
  - 自动读取并缓存数据库解密密钥，无须用户手动填写复杂 Token。

- **谷歌云盘一键同步与云端备份 (Google Drive Cloud Backup)**:
  - 内置 `sync_to_gdrive` 工具，支持一键将微信接收的文件/附件增量同步到谷歌云盘（挂载于 `~/gdrive/wechat`）。
  - 支持指定单个文件或文档直接上传至云盘，自动处理路径与同名去重。

- **严格只读与隐私保护 (Strictly Read-Only)**:
  - 所有 MCP 工具均为 `readOnlyHint: true`，不具备发送、删除或修改消息的权限。
  - 支持可选 `--sanitize` 标志，自动脱敏手机号、身份证、邮箱与银行卡号。

---

## 2. 快速编译与运行 (Build & Run)

### Linux 平台
```bash
# 编译原生二进制 (需 CGO)
go build -o bin/wechat_mcp ./cmd/wechat_mcp

# 运行自检诊断
./bin/wechat_mcp --doctor

# 生成 Claude Desktop 配置
./bin/wechat_mcp --install claude-desktop
```

### Windows 平台 (PowerShell / CMD)
```powershell
# 原生编译 (纯 Go，无需 GCC/MinGW)
go build -o bin\wechat_mcp.exe .\cmd\wechat_mcp

# 交叉编译 (在 Linux 上为 Windows 生成)
GOOS=windows go build -o bin/wechat_mcp.exe ./cmd/wechat_mcp

# 运行自检诊断
.\bin\wechat_mcp.exe --doctor

# 生成 MCP 客户端配置
.\bin\wechat_mcp.exe --install claude-desktop
```

---

## 3. 微信数据库解密密钥获取与自动化配置 (Database Key Extraction)

微信本地 SQLite 数据库（`session.db`、`message_0.db`、`contact.db`）采用 256-bit AES 进行加密。`wechat_mcp` 提供了**一键自动化提取脚本**与完善的兜底方案：

### 方案 A：Windows 一键全自动内存提取脚本（强烈推荐）
仓库内内置了自动化内存探测与密钥注入脚本 `scripts/get_key_windows.py`（以及 PowerShell 封装 `scripts/get_key_windows.ps1`）：
1. 确保 Windows 微信处于登录并运行状态。
2. 运行脚本：
   ```powershell
   # 方式 1：Python 一键运行（自动检测并安装必要依赖，自动扫描微信内存）
   python scripts\get_key_windows.py

   # 方式 2：PowerShell 运行
   .\scripts\get_key_windows.ps1
   ```
3. 脚本会自动提取 64 位十六进制密钥，并**自动创建并写入配置文件** `%APPDATA%\wechat_mcp\config.json`：
   ```json
   {
     "decryptKey": "0bdb5000f4914f49b30fad48b5ad23c252f4df60206641919117d6b9aa40aff7",
     "wxid": "wxid_xxxx"
   }
   ```
4. 随后运行 `.\bin\wechat_mcp.exe --doctor` 即可见证 `[OK] Loaded` 与全绿诊断！

### 方案 B：WeFlow 零密钥免提取桥接（自动兜底）
若不想从内存中扫描密钥，且本地安装有 WeFlow：
- 只需在 WeFlow 设置中开启 HTTP API（默认端口 `5031`）。
- `wechat_mcp` 具有双模自适应引擎：当原生 WCDB 密钥未配置时，会自动感知并无缝切换至 WeFlow 本地高速 HTTP 桥接，无需任何密钥配置。

### 方案 C：手动配置或环境变量注入
若您通过其他工具（如 PyWxDump 命令行 `pywxdump bias --auto` 或 Cheat Engine 等）已取得 64 位密钥：
- 可设置环境变量：`$env:WECHAT_DB_KEY="你的64位密钥"`
- 或手动保存至配置文件：`%APPDATA%\wechat_mcp\config.json` (Windows) 或 `~/.config/wechat_mcp/config.json` (Linux)。

---

## 4. 给 Agent 自行配置的提示词 (Agent System Prompts & Instructions)

> **请将以下内容直接复制并提供给你的 AI Agent（如 Claude Desktop、Claude Code、Cursor、Windsurf），Agent 将自动了解如何使用本 MCP 服务为你服务：**

```markdown
# 微信个人助理扩展协议指令 (WeChat MCP Assistant Instructions)

你现在已通过 Model Context Protocol (MCP) 连接到了用户的本地 Linux 微信客户端。你拥有只读访问用户微信聊天记录、联系人以及已接收文件的能力。

## 可用工具清单及使用规范

1. `list_sessions`:
   - 用途: 列出用户的近期会话列表（单聊、群聊、企业微信、公众号）。
   - 参数:
     - `limit` (int): 返回条数 (1-200, 默认 50)。
     - `offset` (int): 分页偏移。
     - `keyword` (string, 可选): 过滤会话名称或用户名。
   - 实践: 当用户询问“我最近有哪些未读消息”、“最近谁联系过我”或想要了解聊天全局时首先调用。

2. `list_messages`:
   - 用途: 读取指定会话中的消息记录。
   - 参数:
     - `sessionId` (string, 必需): 会话 ID（如 `wxid_xxx` 或 `xxx@chatroom`）。
     - `limit` (int): 获取条数 (默认 50)。
     - `order` (string): `"desc"`(从新到旧) 或 `"asc"`(从旧到新)。
     - `since` (int, 可选): 起始秒级时间戳。
     - `end` (int, 可选): 截止秒级时间戳。
   - 特别注意: 当消息为文件传输 (Type 49) 时，结果中会自动携带 `filePath`、`fileName`、`fileSize`。你可以直接使用文件读取工具打开该 `filePath` 进行阅读与分析。

3. `search_messages`:
   - 用途: 按关键字全局检索聊天记录，或限定在某个会话内搜索。
   - 参数:
     - `keyword` (string, 必需): 搜索词。
     - `sessionId` (string, 可选): 限定特定群聊或好友。
     - `since` / `end` (int, 可选): 时间范围。

4. `get_contact`:
   - 用途: 获取联系人或群聊的详细信息（真实昵称、备注、微信号、群名称）。
   - 参数: `username` (string, 必需)。

5. `list_files`:
   - 用途: 浏览或搜索微信在本地保存的所有文件（PDF、Word、Excel、PPT、压缩包等）。
   - 参数:
     - `keyword` (string, 可选): 文件名关键字。
     - `ext` (string, 可选): 文件扩展名（如 `"pdf"`、`"docx"`、`"xlsx"`）。
     - `limit` / `offset` (int): 分页。

6. `get_message_file`:
   - 用途: 根据文件名精确定位该文件在 Linux 系统中的绝对物理路径。
   - 参数: `fileName` (string, 必需)。
   - 实践: 获得返回的 `filePath` 后，直接调用你的文件阅读工具提取文件内容，回答用户针对具体文档的提问。

7. `sync_to_gdrive`:
   - 用途: 一键将微信本地接收的所有文件/附件增量同步备份至谷歌云盘（`~/gdrive/wechat`），或上传指定文件。
   - 参数:
     - `filePath` (string, 可选): 指定上传的本地文件物理路径或文件名。若为空则全量增量同步所有接收的文件。
     - `targetDir` (string, 可选): 谷歌云盘目标目录，默认为 `/home/sesley/gdrive/wechat`。
     - `overwrite` (bool, 可选): 是否强制覆盖已存在的文件（默认 `false`，同名且大小一致自动跳过）。
   - 实践: 当用户询问“帮我把微信文件备份到谷歌云盘”或“把微信收到的某份文件传到云盘”时直接调用。

## 典型工作流示例

- **场景 A：用户询问“帮我看看群聊/老师今天发了什么文件并总结一下”**：
  1. 调用 `list_messages` 或 `list_files` 找到对应的文件名称与 `filePath`。
  2. 使用你的文件读取工具打开该 `filePath`（如 PDF 或 Word 文档）。
  3. 为用户提炼该文件的核心要点。

- **场景 B：用户询问“今天我和张三聊了什么”**：
  1. 调用 `list_sessions` 找到“张三”对应的 `sessionId`。
  2. 计算今天凌晨的 Unix 时间戳，传入 `list_messages` 的 `since` 参数获取今天所有消息。
  3. 结构化梳理双方的沟通脉络。

- **场景 C：查找历史资料“之前别人在微信给我发过一份排班表/合同在哪”**：
  1. 调用 `get_message_file`（如 `{"fileName": "排班表"}`）。
  2. 将获取到的绝对路径直接呈现或加载。
```

---

## 4. MCP 客户端配置 (Client Configurations)

### Claude Desktop (`~/.config/Claude/claude_desktop_config.json`)

```json
{
  "mcpServers": {
    "wechat": {
      "command": "/absolute/path/to/wechat_mcp/bin/wechat_mcp",
      "args": []
    }
  }
}
```

### Cursor (`.cursor/mcp.json`)

```json
{
  "mcpServers": {
    "wechat": {
      "command": "/absolute/path/to/wechat_mcp/bin/wechat_mcp",
      "args": []
    }
  }
}
```

### Claude Code CLI

```bash
claude mcp add wechat /absolute/path/to/wechat_mcp/bin/wechat_mcp
```

*(如果需要启用手机号/身份证等敏感隐私自动打码脱敏，在 `"args"` 中加入 `"--sanitize"` 即可)*

---

## 5. 可用工具协议规格 (Tool Schemas)

| 工具名称 | 功能描述 | 核心输入参数 |
| :--- | :--- | :--- |
| `list_sessions` | 列出会话列表（排序、未读数、分页） | `limit`, `offset`, `keyword` |
| `list_messages` | 查询指定会话的聊天记录（支持文件路径联动） | `sessionId`, `limit`, `offset`, `order`, `since`, `end` |
| `search_messages` | 全局或会话内关键字搜消息 | `keyword`, `sessionId`, `limit`, `offset` |
| `get_contact` | 查询联系人/群聊信息 | `username` |
| `list_files` | 索引本地接收的 PDF/Word/Excel 等文件 | `keyword`, `ext`, `limit`, `offset` |
| `get_message_file` | 获取特定文件的本机绝对物理路径 | `fileName` |
| `sync_to_gdrive` | 微信文件一键同步或指定文件上传至谷歌云盘 | `filePath`, `targetDir`, `overwrite` |

---

## 6. 免责声明与边界 (Security & Non-Goals)

- 本工具**严格只读**，不支持发送消息、自动回复、朋友圈点赞等写操作，不具备微信自动化 Bot 行为。
- 所有数据均在用户本地计算机完成解密与查询，**不搭建任何外部网络服务器，不上传任何隐私数据**。
