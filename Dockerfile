# ---------- 构建阶段 ----------
FROM golang:1.21-alpine AS builder

WORKDIR /src

# 先拷贝依赖清单，利用 Docker 层缓存加速重复构建
COPY go.mod ./
RUN go mod download 2>/dev/null || true

# 拷贝源码（.go 文件已在 .dockerignore 中排除测试/文档）
COPY *.go ./

# 纯静态编译：CGO 关闭，去掉调试信息，缩小体积
RUN CGO_ENABLED=0 GOOS=linux go build \
      -trimpath \
      -ldflags "-s -w" \
      -o /out/getapp-danmu .

# ---------- 运行阶段 ----------
FROM alpine:3.19

LABEL org.opencontainers.image.title="logvar-getapp-danmu" \
      org.opencontainers.image.description="LogVar 弹幕 API 对接 Getapp(苹果CMS V10) 的适配服务，支持资源站热更新" \
      org.opencontainers.image.version="1.0.0"

# ca-certificates 必需（HTTPS 请求上游）；tzdata 用于 TZ=Asia/Shanghai
RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S app && adduser -S -G app app

WORKDIR /app

COPY --from=builder /out/getapp-danmu /app/getapp-danmu
COPY docker-entrypoint.sh /app/docker-entrypoint.sh
COPY config.example.json /app/config.example.json

RUN chmod +x /app/getapp-danmu /app/docker-entrypoint.sh && \
    mkdir -p /app/data && \
    chown -R app:app /app

ENV TZ=Asia/Shanghai \
    CONFIG_PATH=/app/data/config.json \
    LISTEN=:12381 \
    ADMIN_ENABLED=false \
    WATCH_INTERVAL_SEC=15

# 配置持久化目录（挂载出来即可热改 config.json）
VOLUME ["/app/data"]

EXPOSE 12381

USER app

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget -qO- http://127.0.0.1:12381/health >/dev/null 2>&1 || exit 1

ENTRYPOINT ["/app/docker-entrypoint.sh"]
CMD ["/app/getapp-danmu"]
