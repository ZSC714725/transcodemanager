# TranscodeManager - 多阶段构建，含 FFmpeg 支持

# 阶段 1: 编译 Go 应用
FROM golang:1.24-alpine AS builder

WORKDIR /build

# 复制依赖声明
COPY go.mod go.sum ./

# 预下载依赖（利用 Docker 缓存）
RUN go mod download

# 复制源码并编译
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o transcodemanager ./cmd/server

# 阶段 2: 运行镜像（FFmpeg 8.0，基于 jrottenberg/ffmpeg）
FROM jrottenberg/ffmpeg:8.0-alpine

WORKDIR /app

ENV GIN_MODE=release

# HTTPS 拉流所需 CA 证书；wget 用于 HEALTHCHECK
RUN apk add --no-cache ca-certificates wget

COPY --from=builder /build/transcodemanager .
COPY --from=builder /build/web ./web
COPY --from=builder /build/config.yaml .

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/health/live > /dev/null || exit 1

ENTRYPOINT ["./transcodemanager"]
CMD ["-config", "/app/config.yaml"]
