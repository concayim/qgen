# qgen · 知识库问题自动生成 Agent

基于 **Golang + [eino](https://github.com/cloudwego/eino)**（字节 CloudWeGo 的大模型应用框架）实现的智能体：
扫描知识库清单（规章制度 / 领导讲话 / 会议纪要 / 工作方案等文档），按**政务意图**为每篇文档自动生成
多类用户提问，并导出为与测试模板一致的 Excel（`轮次 / 场景 / 问题 / 上传文档 / 来源文档 / 答案位置提示`）。

## 特性

- **意图感知出题**：每篇文档按配置的意图分布出题，覆盖三类真实用户诉求：
  - `information_interpretation`（信息解读）：可在原文找到答案的事实型提问；
  - `guidance`（文稿撰写）：以该讲话/会议/文件为依据，撰写一篇新的正式文稿；
  - `draft_from_doc`（以稿写稿）：在问题中嵌入一段真实原文，要求改写/压缩/润色。
  「场景」列即记录了每条问题对应的意图类别。
- **eino 编排**：`compose.Chain[map[string]any, *schema.Message]` 串联 `prompt.ChatTemplate`（Go 模板渲染提示词）与 OpenAI 兼容的 `ChatModel`。
- **广泛兼容**：任意 OpenAI 兼容接口（火山方舟 / 本地 vLLM / OpenAI / 中转端点）；对 gpt-5.x 等强制固定采样参数的 beta 模型做了兼容（`temperature<=0` 时不发送）。
- **并发 + 重试**：worker 池按文档并发调用模型，保留文档顺序，失败自动重试 3 次。
- **稳健解析**：兼容纯 JSON、` ```json ` 代码块包裹、以及前后夹带解释文字的情况。

## 工作流

```
知识库目录
   │  ① 递归扫描，生成「知识库清单」(internal/kb)
   ▼
[]Document ──② 读取正文并按长度截断──▶ 文档正文
   │
   ▼  ③ eino Chain：ChatTemplate ──▶ ChatModel  (internal/gen)
模型输出(按意图分桶的 JSON) ──④ 稳健解析 + 按意图裁剪──▶ []Question
   │
   ▼  ⑤ 全局编号并导出 (internal/export)
知识库问题清单.xlsx
```

## 目录结构

```
qgen/
├── cmd/qgen/main.go          # CLI 入口（扫描 → 生成 → 导出）
├── internal/config/          # 配置加载（yaml + 环境变量）
├── internal/kb/              # 知识库扫描与文档读取
├── internal/gen/             # 基于 eino 的意图感知问题生成器 + 提示词
├── internal/export/          # xlsx 导出（对齐测试模板列）
├── config.example.yaml       # 配置示例
├── 需求文档.md                # 需求与变更记录
└── README.md
```

## 快速开始

### 1. 配置模型

支持任意 **OpenAI 兼容** 接口。推荐用环境变量注入：

```bash
export QGEN_BASE_URL="http://127.0.0.1:8317/v1"   # 端点（注意通常带 /v1）
export QGEN_API_KEY="你的-api-key"                 # 也兼容 ARK_API_KEY / OPENAI_API_KEY
export QGEN_MODEL="gpt-5.5"                        # 模型名 / 方舟 endpoint id
export QGEN_TEMPERATURE="0"                        # gpt-5.x 等 beta 模型需设为 0（不发送 temperature）
```

或复制 `config.example.yaml` 为 `config.yaml` 后修改。

### 2. 预览知识库清单（不调模型）

```bash
go run ./cmd/qgen -dry-run -root ".."
```

### 3. 生成问题

```bash
# 先小批量试跑（最多 5 篇）
go run ./cmd/qgen -root ".." -max 5 -out 试跑.xlsx

# 按类别全量生成
go run ./cmd/qgen -root "../领导讲话" -c 6 -out 领导讲话-问题清单.xlsx
```

编译为可执行文件：

```bash
go build -o qgen ./cmd/qgen
./qgen -config config.yaml
```

## 命令行参数

| 参数 | 说明 | 默认 |
| --- | --- | --- |
| `-config` | yaml 配置文件路径（可选） | 空 |
| `-root` | 知识库根目录，可多次指定 | 配置值 / `.` |
| `-out` | 输出 xlsx 路径 | `知识库问题清单.xlsx` |
| `-n` | 覆盖 `information_interpretation` 意图的条数（其余按配置） | 配置值 |
| `-c` | 并发文档数 | 4 |
| `-max` | 最多处理文档数（0 = 不限） | 0 |
| `-dry-run` | 仅打印清单，不调用模型 | false |

环境变量 `QGEN_BASE_URL / QGEN_API_KEY / QGEN_MODEL / QGEN_TEMPERATURE / QGEN_OUTPUT / QGEN_CONCURRENCY` 可覆盖配置。

## 意图分布配置

在 `config.yaml` 的 `gen.intents` 中配置每篇文档各意图的出题数（默认 3 / 1 / 1）：

```yaml
gen:
  intents:
    - intent: "information_interpretation"
      scene: "信息解读"
      count: 3
    - intent: "guidance"
      scene: "文稿撰写"
      count: 1
    - intent: "draft_from_doc"
      scene: "以稿写稿"
      count: 1
  concurrency: 4
  max_docs: 0
```

## 输入与输出

- **输入**：默认扫描 `.md / .txt / .markdown` 文档（docx/pdf 请先转换为 md）。文档标题优先取正文首个一/二级标题，否则取文件名。
- **输出**：`问题清单` 工作表，列为 `轮次 | 场景 | 问题 | 上传文档 | 来源文档 | 答案位置提示`，与 `文档提问20条-测试模板.xlsx` 对齐；`场景`列体现意图类别。

## 桌面客户端（Electron）

`desktop/` 下提供了一个 Electron 图形客户端：前端负责配置与可视化进度，实际生成仍由 Go 的 `qgen` 二进制完成（通过 `-json` 进度协议通信）。

功能：选择知识库目录、配置模型（Base URL / API Key / 模型 / temperature）、设置三类意图的每篇条数与并发、预览清单、实时进度与日志、完成后一键打开文件或定位文件夹。配置（含 API Key）只保存在本机 `userData/settings.json`，不会进入仓库。

```bash
cd desktop
npm install            # 安装 Electron
npm run build:agent    # 编译 Go 二进制到 desktop/bin/qgen（需本机有 Go）
npm start              # 启动客户端
```

> 若启动后报 `Cannot read properties of undefined (reading 'whenReady')`，说明当前环境设置了 `ELECTRON_RUN_AS_NODE`，用 `env -u ELECTRON_RUN_AS_NODE npm start` 启动即可。

打包安装包（可选）：`npm run dist`（使用 electron-builder，会把 `bin/qgen` 作为 extraResources 一起打包）。

## Docker 部署

容器化的是 Go 命令行 agent（GUI 客户端不适合容器）。镜像采用多阶段构建，运行时基于 alpine，约定知识库挂载到 `/kb`、输出写到 `/out`。

### 构建镜像

```bash
docker build -t qgen .
# 海外网络可指定官方代理：
# docker build --build-arg GOPROXY=https://proxy.golang.org,direct -t qgen .
```

### 运行（一次性任务）

```bash
docker run --rm \
  -e QGEN_BASE_URL="https://api.openai.com/v1" \
  -e QGEN_API_KEY="你的-api-key" \
  -e QGEN_MODEL="gpt-5.5" \
  -e QGEN_TEMPERATURE="0" \
  -v /本机/知识库目录:/kb:ro \
  -v /本机/输出目录:/out \
  qgen -root /kb -out /out/知识库问题清单.xlsx -c 6
```

先预览清单（不调用模型，可不带 key）：

```bash
docker run --rm -v /本机/知识库目录:/kb:ro qgen -dry-run -root /kb
```

> **访问宿主机本地模型服务**：容器内的 `127.0.0.1` 指向容器自身。若模型服务跑在宿主机（如 `127.0.0.1:8317`），请把 `QGEN_BASE_URL` 改为 `http://host.docker.internal:8317/v1`，并在 `docker run` 加 `--add-host=host.docker.internal:host-gateway`（Linux）；macOS/Windows 的 Docker Desktop 默认已支持 `host.docker.internal`。

### docker compose

仓库提供了 `docker-compose.yml`。用环境变量或 `.env` 注入配置后：

```bash
export QGEN_API_KEY="你的-api-key"
export KB_DIR=/本机/知识库目录
export OUT_DIR=/本机/输出目录
docker compose run --rm qgen
```

## 测试

```bash
go test ./...
```

覆盖：模型输出的按意图分桶 JSON 解析（多种格式）、xlsx 导出列与内容校验。
