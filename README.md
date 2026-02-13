# TranscodeManager

基于 Core 项目 FFmpeg 逻辑的转码任务管理服务，负责转码任务的创建、启停与监控，无媒体接入与分发（RTMP/SRT/HTTP 等）。

## 功能

- **FFmpeg 进程管理**：创建、启动、停止、重启转码任务
- **进度解析**：解析 FFmpeg stderr，输出 frame、time、speed、size 等进度
- **CPU/内存监控**：采集运行中任务的实际 CPU 占用与内存使用
- **Skills**：探测 FFmpeg 版本、编解码器、协议、滤镜等能力
- **REST API**：参考 Core 的 `/api/v3/process` 设计

## 环境要求

- **Go** 1.23+
- **FFmpeg**：需在 PATH 或通过配置指定可执行路径

## Docker 运行

项目支持 Docker 部署，镜像内已包含 FFmpeg：

```bash
# 构建并运行（docker-compose）
docker compose up -d --build

# 或仅构建镜像
docker build -t transcodemanager .

# 运行容器
docker run -d -p 8080:8080 -v $(pwd)/data:/data transcodemanager
```

启动后访问 http://localhost:8080。文件转码任务请将输入/输出路径置于挂载的 `/data` 目录下（如 `/data/input.mp4`）。

## 快速开始

```bash
# 构建
go build -o transcodemanager ./cmd/server

# 使用默认配置运行（监听 :8080）
./transcodemanager

# 使用配置文件
./transcodemanager -config config.yaml

# 命令行覆盖
./transcodemanager -bind :9000 -ffmpeg /opt/ffmpeg/bin/ffmpeg
```

启动后访问 http://localhost:8080 使用 Web 控制台。

> 需在项目根目录（含 `web/` 目录）下运行，前端才能正常加载。

## Web 控制台

- **任务列表**：查看所有任务及状态（finished/starting/running 等）
- **添加任务**：填写输入/输出、输入选项、转码选项、输出选项
- **编辑任务**：修改已创建任务的配置
- **启停控制**：启动、停止、重启、删除（添加后默认不自动启动，需手动启动）
- **状态**：运行时长、CPU、内存、FFmpeg 进度（帧数、速度、已处理时长、输出大小）
- **命令**：查看生成的完整 FFmpeg 命令
- **日志**：查看 FFmpeg stderr 输出（含 frame/speed 等 progress 行）

## FFmpeg 命令生成规则

命令结构：`ffmpeg [输入选项] -i [输入地址] [输出选项] [输出地址]`

- **输入选项**：放在 `-i` 前，如 `-re -stream_loop -1`
- **转码选项**：音视频编解码，放在输出地址前，如 `-vcodec copy -acodec copy`，默认 `-c:v libx264 -c:a aac`
- **输出选项**：输出格式等，如 `-f flv`

示例（RTMP 拉流转推）：

```bash
ffmpeg -re -stream_loop -1 -i rtmp://live.example.com/stream \
  -vcodec copy -acodec copy -f flv rtmp://publish.example.com/push
```

## API 参考

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/v3/skills | FFmpeg 能力列表 |
| POST | /api/v3/skills/reload | 重新加载能力 |
| GET | /api/v3/process | 任务列表 |
| POST | /api/v3/process | 添加任务 |
| GET | /api/v3/process/:id | 任务详情 |
| PUT | /api/v3/process/:id | 更新任务 |
| DELETE | /api/v3/process/:id | 删除任务 |
| GET | /api/v3/process/:id/config | 配置 |
| GET | /api/v3/process/:id/state | 状态与进度 |
| GET | /api/v3/process/:id/report | 日志 |
| PUT | /api/v3/process/:id/command | start / stop / restart |

### 添加任务（文件转码）

```bash
curl -X POST http://localhost:8080/api/v3/process \
  -H "Content-Type: application/json" \
  -d '{
    "input": [{"address": "/path/to/input.mp4"}],
    "output": [{
      "address": "/path/to/output.mp4",
      "options": ["-c:v", "libx264", "-c:a", "aac"]
    }],
    "autostart": false
  }'
```

### 添加任务（RTMP 拉流转推）

```bash
curl -X POST http://localhost:8080/api/v3/process \
  -H "Content-Type: application/json" \
  -d '{
    "input": [{
      "address": "rtmp://live.example.com/app/stream",
      "options": ["-re", "-stream_loop", "-1"]
    }],
    "output": [{
      "address": "rtmp://publish.example.com/app/push",
      "options": ["-vcodec", "copy", "-acodec", "copy", "-f", "flv"]
    }],
    "autostart": false
  }'
```

### 启动 / 停止 / 重启

```bash
# 启动
curl -X PUT http://localhost:8080/api/v3/process/{id}/command \
  -H "Content-Type: application/json" \
  -d '{"command": "start"}'

# 停止
curl -X PUT http://localhost:8080/api/v3/process/{id}/command \
  -H "Content-Type: application/json" \
  -d '{"command": "stop"}'

# 重启
curl -X PUT http://localhost:8080/api/v3/process/{id}/command \
  -H "Content-Type: application/json" \
  -d '{"command": "restart"}'
```

## 配置

通过 `-config` 指定 YAML 配置文件（可选）：

```bash
./transcodemanager -config config.yaml
```

`config.yaml` 示例：

```yaml
server:
  bind: ":8080"          # 服务监听地址，如 ":8080" 或 "0.0.0.0:8080"

ffmpeg:
  path: "ffmpeg"         # FFmpeg 可执行路径
                         # - "ffmpeg": 从系统 PATH 查找
                         # - 完整路径: "/usr/bin/ffmpeg"
```

命令行参数可覆盖配置：`-bind`、`-ffmpeg`。

## 项目结构

```
transcodemanager/
├── cmd/server/          # 主程序入口
├── internal/
│   ├── api/             # REST API 与 handlers
│   ├── config/          # 配置加载
│   ├── ffmpeg/          # FFmpeg 封装、parser、skills、validator
│   ├── logger/          # 日志
│   ├── process/         # 进程控制、limiter
│   └── task/            # 任务与 Store
├── web/                 # 前端静态资源
│   └── index.html
├── config.yaml          # 配置示例
└── README.md
```
