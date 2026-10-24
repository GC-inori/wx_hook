#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
SOCKET_PATH="${WX_BRIDGE_SOCKET:-/tmp/wx_bridge.sock}"
BRIDGE_LIB="${WX_BRIDGE_LIBRARY:-$ROOT_DIR/librust_bridge.so}"
BOT_BIN="$ROOT_DIR/wxbot"
HTTP_PORT="${HTTP_PORT:-8899}"

# 1. 自动探测微信可执行文件路径
if [[ -z "${WECHAT_BIN:-}" ]]; then
  for candidate in "/opt/wechat/wechat" "/usr/bin/wechat" "/opt/wechat-beta/wechat"; do
    if [[ -x "$candidate" ]]; then
      WECHAT_BIN="$candidate"
      break
    fi
  done
fi
WECHAT_BIN="${WECHAT_BIN:-/opt/wechat/wechat}"

echo "=== 微信 Linux 机器人启动器 ==="

# 2. 检查 --check 参数 (仅语法与配置检查)
if [[ "${1:-}" == "--check" || "${1:-}" == "-check" ]]; then
  if [[ -x "$BOT_BIN" ]]; then
    "$BOT_BIN" -root "$ROOT_DIR" -check
    exit 0
  fi
fi

# 3. 检查微信可执行文件与动态库
if [[ ! -x "$WECHAT_BIN" ]]; then
  echo "[错误] 未找到微信可执行文件: $WECHAT_BIN" >&2
  echo "请先安装微信 deb 包: sudo apt install -y ./WeChatLinux_x86_64.deb" >&2
  exit 1
fi

if [[ ! -f "$BRIDGE_LIB" ]]; then
  echo "[错误] 未找到注入动态库: $BRIDGE_LIB" >&2
  exit 1
fi

if [[ ! -f "$BOT_BIN" ]]; then
  echo "[错误] 未找到机器人程序: $BOT_BIN" >&2
  exit 1
fi
chmod +x "$BOT_BIN"

# 4. 获取本机局域网 IP
HOST_IP=$(hostname -I 2>/dev/null | awk '{print $1}' || echo "127.0.0.1")

# 5. 运行模式策略：默认强制走 Xvfb 虚拟桌面，防止 SSH X11 转发误判导致界面失踪
# 只有显式传入 --gui 且 DISPLAY 非空时才尝试物理窗口
USE_XVFB=1
if [[ "${GUI:-0}" == "1" || "${1:-}" == "--gui" ]]; then
  if [[ -n "${DISPLAY:-}" ]]; then
    USE_XVFB=0
    echo "[启动] 显式指定 --gui 模式，尝试使用物理图形环境 ($DISPLAY)..."
  fi
fi

if [[ "$USE_XVFB" -eq 1 ]]; then
  if ! command -v Xvfb >/dev/null 2>&1; then
    echo "[错误] 未找到 Xvfb，请执行以下命令安装运行依赖:" >&2
    echo "sudo apt update && sudo apt install -y xvfb fonts-noto-cjk scrot zbar-tools qrencode" >&2
    exit 1
  fi
fi

# 6. 创建统一日志目录并彻底清理残留的旧套接字
mkdir -p "$ROOT_DIR/logs"
export WX_BRIDGE_LOG_DIR="$ROOT_DIR/logs"
rm -f "$SOCKET_PATH"

# 7. 启动微信进程并注入 Bridge
WECHAT_LOG="$ROOT_DIR/logs/wechat.log"
WECHAT_ENV=(
  env
  LD_PRELOAD="$BRIDGE_LIB"
  WX_BRIDGE_SOCKET="$SOCKET_PATH"
  WX_BRIDGE_LOG_DIR="$ROOT_DIR/logs"
  QT_XCB_GL_INTEGRATION=none
)

WECHAT_PID=""
HTTP_PID=""
XVFB_PID=""

cleanup() {
  echo ""
  echo "[退出] 正在停止后台微信与关联服务..."
  if [[ -n "${WECHAT_PID:-}" ]] && kill -0 "$WECHAT_PID" 2>/dev/null; then
    kill -TERM "$WECHAT_PID" 2>/dev/null || true
    wait "$WECHAT_PID" 2>/dev/null || true
  fi
  if [[ -n "${HTTP_PID:-}" ]] && kill -0 "$HTTP_PID" 2>/dev/null; then
    kill -TERM "$HTTP_PID" 2>/dev/null || true
  fi
  if [[ -n "${XVFB_PID:-}" ]] && kill -0 "$XVFB_PID" 2>/dev/null; then
    kill -TERM "$XVFB_PID" 2>/dev/null || true
    rm -f /tmp/.X99-lock /tmp/.X11-unix/X99
  fi
  rm -f "$SOCKET_PATH"
}
trap cleanup EXIT INT TERM

if [[ "$USE_XVFB" -eq 1 ]]; then
  echo "[启动] 使用 Xvfb 虚拟桌面模式启动微信 (Xvfb :99)..."
  rm -f /tmp/.X99-lock /tmp/.X11-unix/X99
  Xvfb :99 -ac -screen 0 1280x1024x24 -nolisten tcp >/dev/null 2>&1 &
  XVFB_PID=$!
  sleep 0.5
  DISPLAY=:99 "${WECHAT_ENV[@]}" "$WECHAT_BIN" >"$WECHAT_LOG" 2>&1 &
  WECHAT_PID=$!
else
  "${WECHAT_ENV[@]}" "$WECHAT_BIN" >"$WECHAT_LOG" 2>&1 &
  WECHAT_PID=$!
fi

# 8. 等待并严密校验套接字连通性
echo "[等待] 等待 Rust Bridge 通信套接字就绪 ($SOCKET_PATH)..."
READY=0
for ((i=1; i<=40; i++)); do
  if ! kill -0 "$WECHAT_PID" 2>/dev/null; then
    echo "[错误] 微信主进程意外退出 最后日志输出如下 (详见 $WECHAT_LOG):" >&2
    echo "--------------------------------------------------------" >&2
    tail -n 25 "$WECHAT_LOG" >&2 || true
    echo "--------------------------------------------------------" >&2
    exit 1
  fi

  if [[ -S "$SOCKET_PATH" ]]; then
    if python3 -c "import socket; s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM); s.connect('$SOCKET_PATH'); s.close()" 2>/dev/null; then
      READY=1
      break
    fi
  fi
  sleep 0.5
done

if [[ "$READY" -ne 1 ]]; then
  echo "[错误] 等待桥接套接字超时 请检查日志:" >&2
  echo "  微信日志: cat $WECHAT_LOG" >&2
  echo "  桥接日志: cat $ROOT_DIR/logs/bridge.log" >&2
  exit 1
fi

# 9. 在虚拟桌面模式下自动启动扫码预览服务
if [[ "$USE_XVFB" -eq 1 ]]; then
  # 后台启动临时 HTTP 图片服务器
  if python3 -c "import socket; s = socket.socket(); s.connect(('127.0.0.1', $HTTP_PORT)); s.close()" 2>/dev/null; then
    echo "[提示] 端口 $HTTP_PORT 已有服务监听"
  else
    python3 -m http.server "$HTTP_PORT" --directory "$ROOT_DIR" >/dev/null 2>&1 &
    HTTP_PID=$!
  fi

  # 循环自动检测二维码并在控制台打印
  (
    sleep 2.0
    # 若存在历史授权，自动按下 Return 触发“确认登录”
    if command -v xdotool >/dev/null 2>&1; then
      DISPLAY=:99 xdotool key Return 2>/dev/null || true
    fi

    qr_found=0
    for retry in {1..12}; do
      sleep 1.0
      if [[ -n "${WECHAT_PID:-}" ]] && ! kill -0 "$WECHAT_PID" 2>/dev/null; then
        break
      fi
      DISPLAY=:99 scrot -o "$ROOT_DIR/qrcode.png" 2>/dev/null || true
      if [[ -f "$ROOT_DIR/qrcode.png" ]] && command -v zbarimg >/dev/null 2>&1; then
        qr_url=$(zbarimg -q --raw "$ROOT_DIR/qrcode.png" 2>/dev/null | head -n 1 || true)
        if [[ -n "$qr_url" ]]; then
          qr_found=1
          echo ""
          echo "==================== [控制台扫码登录] ===================="
          if command -v qrencode >/dev/null 2>&1; then
            qrencode -t UTF8 "$qr_url"
          fi
          echo "=========================================================="
          echo "请直接用手机微信扫描上方二维码（务必勾选'自动登录'）"
          echo "若终端字符排版变形，可直接在浏览器打开图片扫码:"
          echo "  http://${HOST_IP}:${HTTP_PORT}/qrcode.png"
          echo "=========================================================="
          echo ""
          break
        fi
      fi
    done

    if [[ "$qr_found" -eq 0 && -d "/root/xwechat_files" ]]; then
      echo ""
      echo "==================== [快捷免扫码登录] ===================="
      echo "检测到历史授权凭据，已自动向手机发送【登录确认】推送！"
      echo "请直接在手机微信中点击【登录】确认即可进入主界面。"
      echo "=========================================================="
      echo ""
    fi
  ) &
fi

echo "[成功] 桥接套接字已就绪，启动 Go 机器人客户端..."
export WX_BRIDGE_LIBRARY="$BRIDGE_LIB"
export WX_BRIDGE_PRELOADED=1
"$BOT_BIN" -root "$ROOT_DIR" -dll "$BRIDGE_LIB"
