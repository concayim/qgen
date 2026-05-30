# syntax=docker/dockerfile:1

# ---------- 构建阶段 ----------
FROM golang:1.25-alpine AS builder

# 国内构建加速；如在海外可在 build 时用 --build-arg GOPROXY=https://proxy.golang.org,direct 覆盖
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY} \
    CGO_ENABLED=0 \
    GOOS=linux

WORKDIR /src

# 先拷依赖清单并下载，利用层缓存
COPY go.mod go.sum ./
RUN go mod download

# 再拷源码构建（desktop/、xlsx 等已由 .dockerignore 排除）
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/qgen ./cmd/qgen

# ---------- 运行阶段 ----------
FROM alpine:3.20

# ca-certificates 用于访问 HTTPS 模型端点；tzdata 提供时区
RUN apk add --no-cache ca-certificates tzdata && update-ca-certificates

WORKDIR /data
COPY --from=builder /out/qgen /usr/local/bin/qgen

# 约定：知识库挂载到 /kb，输出写到 /out
VOLUME ["/kb", "/out"]

ENTRYPOINT ["qgen"]
# 默认打印帮助；实际使用时在 docker run 后追加参数覆盖
CMD ["-h"]
