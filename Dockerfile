# Stage 1: Builder - 编译 Go 应用程序
# 使用官方 Go 镜像作为构建环境，该镜像包含了 Go SDK 和 C/C++ 编译器 (gcc)
FROM golang:1.25 AS builder

# 设置工作目录
WORKDIR /app

# 安装 CGO 所需的 C 语言开发库
# gocc 依赖 OpenCC C 库，go-sqlite3 依赖 SQLite C 库
# libopencc-dev 提供 OpenCC 的头文件和静态库，用于编译 gocc
# libsqlite3-dev 提供 SQLite3 的头文件和静态库，用于编译 go-sqlite3
# --no-install-recommends 减少额外不必要的包安装
RUN apt-get update && apt-get install -y \
    libopencc-dev \
    libsqlite3-dev \
    --no-install-recommends \
    && rm -rf /var/lib/apt/lists/*

# 复制 go.mod 和 go.sum 文件
# 这一步单独进行，如果这两个文件没有变化，Docker 会使用缓存层，无需重新下载依赖
COPY go.mod ./
COPY go.sum ./

# 下载 Go 模块依赖
# 如果 go.mod 或 go.sum 发生变化，此命令会重新执行
RUN go mod download

# 复制项目所有源代码到容器中
# 这一步通常是触发构建缓存失效的关键点，所以放在模块下载之后
COPY . .

# 构建 Go 应用程序
# CGO_ENABLED=1 确保 CGO 依赖被正确编译和链接
# -o music-processor 指定输出可执行文件名为 music-processor
# -ldflags "-s -w" 移除调试信息和符号表，进一步减小可执行文件大小
# ./cmd/music-processor 指定 main 包的路径
RUN CGO_ENABLED=1 go build -o music-processor -ldflags "-s -w" ./cmd/music-processor

# Stage 2: Runner - 创建最终的最小运行镜像
# 使用 debian:bookworm-slim 作为基础镜像，它是一个非常小的 Debian 发行版
FROM debian:bookworm-slim

# 安装 FFmpeg 和 CGO 运行时所需的 C 语言库
# ffmpeg 包本身提供了 /usr/bin/ffmpeg 可执行文件
# libopencc1g 是 OpenCC 的运行时库，libsqlite3-0 是 SQLite3 的运行时库
# --no-install-recommends 减少不必要的包
RUN apt-get update && apt-get install -y \
    ffmpeg \
    libopencc1g \
    libsqlite3-0 \
    --no-install-recommends \
    && rm -rf /var/lib/apt/lists/*

# 创建一个非 root 用户来运行应用程序，提高安全性
RUN groupadd --system appuser && useradd --system --gid appuser appuser
USER appuser

# 设置应用程序的工作目录
WORKDIR /app

# 从 builder 阶段复制编译好的可执行文件
# /usr/local/bin 是 PATH 环境变量包含的目录，方便直接执行
COPY --from=builder /app/music-processor /usr/local/bin/music-processor

# 定义数据卷，用于持久化存储音乐文件和数据库
# Docker Mounts 会覆盖这些路径下的内容
VOLUME /app/download
VOLUME /app/music
VOLUME /app/data

# 设置启动命令，运行编译好的应用程序
ENTRYPOINT ["/usr/local/bin/music-processor"]

# 可选：设置 CMD 默认参数，如果 ENTRYPOINT 不带任何参数
# CMD []
