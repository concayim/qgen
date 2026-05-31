# Changelog

本项目所有重要变更都记录在此文件。

格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/)。
约定：自此版本起，每次代码改动都提交并推送到 GitHub，并在此追加一条记录。

## [v8] - 2026-05-31

### Changed

- 「上传文件萃取」场景的输出「上传文档」列改为标注**来源文档名**（用户上传了该文档）；其余场景仍为 `无（知识库检索）`。

## [v7] - 2026-05-31

### Changed

- 意图体系由 3 类重构为 **4 个场景**，与业务侧口径对齐：
  - `intelligent_qa`（智能问答）：自然语言事实问答，需检索定位，答案在原文。
  - `draft_from_doc`（以稿写稿）：问题内嵌真实原文片段 + 加工指令。
  - `doc_extraction`（上传文件萃取）：从本篇文档直接抽取/列出/归集成清单或表格。
  - `info_writing`（信息撰写）：依据文档撰写新的正式文稿。
- 重写 `internal/gen/prompt.go` 系统提示，强调「智能问答 vs 上传文件萃取」的区分（开放式问答 vs 定向抽取归集）。
- `internal/config/config.go` 默认意图分布改为 `智能问答4 / 以稿写稿2 / 上传文件萃取2 / 信息撰写2`（每篇 10 条，偏重智能问答）。
- `cmd/qgen` CLI 覆盖参数同步调整：`-n`(智能问答) / `-draft`(以稿写稿) / `-extract`(上传文件萃取) / `-writing`(信息撰写)。
- 更新 `config.example.yaml` 与 `internal/gen/gen_test.go` 以匹配新场景键。

## [v6] - 2026-05-30

### Added

- 多阶段 `Dockerfile`（`golang:1.25-alpine` 构建纯静态二进制，`alpine` 运行）、`.dockerignore`、`docker-compose.yml`。
- README 增加 Docker 构建/运行/compose 命令，说明容器访问宿主机本地模型需用 `host.docker.internal`。

## [v5] - 2026-05-30

### Added

- Electron 桌面客户端（`desktop/`）：复用 Go `qgen` 二进制，经新增的 `-json` 进度协议通信；支持配置、预览、实时进度与结果管理。
- Go 端新增 `-json` 事件输出模式与按意图条数覆盖参数。

## [v4] - 2026-05-30

### Changed

- 导出列精简为 `轮次 / 场景 / 问题 / 上传文档 / 来源文档 / 答案位置提示`，移除 `primary_intent`、`need_retrieval`。
- 更新 README，新增 `需求文档.md`，初始化 git 并推送到 `git@github.com:concayim/qgen.git`。

## [v3] - 2026-05-29

### Added

- 引入政务意图分类，按意图分桶出题；`draft_from_doc` 要求在问题中嵌入真实原文片段。
- 配置改为 `gen.intents`（intent / scene / count 可配）。

## [v2] - 2026-05-29

### Fixed

- 兼容 gpt-5.x 等 beta 模型强制 `temperature=1`：新增 `QGEN_TEMPERATURE`，`<=0` 时不发送该参数。
- 环境变量配置 `BaseURL / APIKey / Model`，兼容 `ARK_API_KEY / OPENAI_API_KEY`；本地 OpenAI 兼容端点需带 `/v1`。

## [v1] - 2026-05-29

### Added

- 基于 Golang + eino 的知识库问题自动生成 agent：扫描知识库（`internal/kb`）、eino Chain 生成（`internal/gen`）、excelize 导出（`internal/export`）、yaml + 环境变量配置（`internal/config`）、并发 worker 池与 `-dry-run` CLI（`cmd/qgen`）。
