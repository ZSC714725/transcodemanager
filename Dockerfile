# TranscodeManager - 多阶段构建，含 FFmpeg 支持

# 阶段 1: 编译 Go 应用
FROM golang:1.23-alpine AS builder

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

# 从 builder 复制二进制
COPY --from=builder /build/transcodemanager .
# 复制前端静态资源
COPY --from=builder /build/web ./web
# 可选：复制配置示例
COPY --from=builder /build/config.yaml .

EXPOSE 8080

# 容器内监听 0.0.0.0 便于外部访问
ENTRYPOINT ["./transcodemanager"]
CMD ["-bind", "0.0.0.0:8080", "-ffmpeg", "ffmpeg"]
