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

## 测试

```bash
go test ./...
```

覆盖：模型输出的按意图分桶 JSON 解析（多种格式）、xlsx 导出列与内容校验。
