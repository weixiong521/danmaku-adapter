#!/bin/sh
# 容器入口脚本：确保配置目录存在；若没有 config.json 则由示例生成一份。
set -e

CONFIG_PATH="${CONFIG_PATH:-/app/data/config.json}"
CONFIG_DIR="$(dirname "$CONFIG_PATH")"

mkdir -p "$CONFIG_DIR"

if [ ! -f "$CONFIG_PATH" ]; then
  if [ -f /app/config.example.json ]; then
    cp /app/config.example.json "$CONFIG_PATH"
    echo "[entrypoint] 未找到配置，已由 config.example.json 生成：$CONFIG_PATH"
  else
    echo "[entrypoint] 警告：未找到 $CONFIG_PATH，且缺少 config.example.json，将使用内置默认值"
  fi
else
  echo "[entrypoint] 使用已有配置：$CONFIG_PATH"
fi

if [ -z "${LISTEN:-}" ] && [ -n "${PORT:-}" ]; then
  case "$PORT" in
    :*) export LISTEN="$PORT" ;;
    *)  export LISTEN=":$PORT" ;;
  esac
fi

echo "[entrypoint] 启动服务：CONFIG_PATH=$CONFIG_PATH LISTEN=${LISTEN:-(默认 :12381)} ADMIN_ENABLED=${ADMIN_ENABLED:-false}"

# 把 PID 1 交给本服务进程，保证能正确接收 SIGTERM 优雅退出
exec "$@"
