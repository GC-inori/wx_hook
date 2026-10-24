#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
SOCKET_PATH="${WX_BRIDGE_SOCKET:-/tmp/wx_bridge.sock}"
DEFAULT_BRIDGE_LIB="$ROOT_DIR/rust_bridge/target/release/librust_bridge.so"
BRIDGE_LIB="${WX_BRIDGE_LIBRARY:-$DEFAULT_BRIDGE_LIB}"
BOT_BIN="$ROOT_DIR/go_bot/go_bot"
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

# 2. 检查 prebuilt 或编译
if [[ ! -f "$BRIDGE_LIB" && -f "$ROOT_DIR/dist_linux/librust_bridge.so" ]]; then
  BRIDGE_LIB="$ROOT_DIR/dist_linux/librust_bridge.so"
fi
if [[ ! -x "$BOT_BIN" && -x "$ROOT_DIR/dist_linux/wxbot" ]]; then
  BOT_BIN="$ROOT_DIR/dist_linux/wxbot"
fi

if [[ "${1:-}" == "--check" || "${1:-}" == "-check" ]]; then
  if [[ -x "$BOT_BIN" ]]; then
    "$BOT_BIN" -root "$ROOT_DIR" -check
    exit 0
  fi
fi

if [[ ! -x "$WECHAT_BIN" ]]; then
  echo "WeChat executable not found or not executable: $WECHAT_BIN" >&2
  exit 1
fi

if [[ ! -f "$BRIDGE_LIB" ]]; then
  if ! command -v cargo >/dev/null 2>&1; then
    echo "Rust bridge not found at $BRIDGE_LIB and cargo is not installed to build it." >&2
    echo "Please provide prebuilt librust_bridge.so or install cargo." >&2
    exit 1
  fi
  echo "Building Rust bridge..."
  (cd "$ROOT_DIR/rust_bridge" && cargo build --release)
fi

if [[ ! -x "$BOT_BIN" ]]; then
  if ! command -v go >/dev/null 2>&1; then
    echo "go_bot not found at $BOT_BIN and go is not installed to build it." >&2
    echo "Please provide prebuilt wxbot or install go." >&2
    exit 1
  fi
  echo "Building go_bot..."
  (cd "$ROOT_DIR/go_bot" && go build -o "$BOT_BIN" .)
fi

HOST_IP=$(hostname -I 2>/dev/null | awk '{print $1}' || echo "127.0.0.1")

is_display_active() {
  local disp="${DISPLAY:-}"
  if [[ -z "$disp" ]]; then
    return 1
  fi
  if [[ "$disp" =~ (localhost|127\.0\.0\.1):([0-9]+) ]]; then
    local dnum="${BASH_REMATCH[2]}"
    local port=$((6000 + dnum))
    if ! python3 -c "import socket; s = socket.socket(); s.settimeout(0.3); s.connect(('127.0.0.1', $port)); s.close()" 2>/dev/null; then
      return 1
    fi
  elif [[ "$disp" =~ ^:([0-9]+) ]]; then
    local sock="/tmp/.X11-unix/X${BASH_REMATCH[1]}"
    if [[ ! -S "$sock" ]]; then
      return 1
    fi
  fi
  return 0
}

USE_XVFB=0
if [[ "${HEADLESS:-0}" == "1" || "${1:-}" == "--headless" || -z "${DISPLAY:-}" ]]; then
  USE_XVFB=1
elif ! is_display_active; then
  echo "[检测] 外部图形环境 ($DISPLAY) 无法连接，自动切换至 Xvfb 虚拟桌面模式..."
  USE_XVFB=1
fi

if [[ "$USE_XVFB" -eq 1 ]]; then
  if ! command -v xvfb-run >/dev/null 2>&1; then
    echo "xvfb-run is required for headless mode; run: sudo apt install -y xvfb fonts-noto-cjk libasound2 libnss3 scrot" >&2
    exit 1
  fi
fi

mkdir -p "$ROOT_DIR/logs"
export WX_BRIDGE_LOG_DIR="$ROOT_DIR/logs"
rm -f "$SOCKET_PATH"

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

cleanup() {
  if [[ -n "${WECHAT_PID:-}" ]] && kill -0 "$WECHAT_PID" 2>/dev/null; then
    kill -TERM "$WECHAT_PID" 2>/dev/null || true
    wait "$WECHAT_PID" 2>/dev/null || true
  fi
  if [[ -n "${HTTP_PID:-}" ]] && kill -0 "$HTTP_PID" 2>/dev/null; then
    kill -TERM "$HTTP_PID" 2>/dev/null || true
  fi
  rm -f "$SOCKET_PATH"
}
trap cleanup EXIT INT TERM

if [[ "$USE_XVFB" -eq 1 ]]; then
  echo "Starting WeChat with Xvfb headless display (Xvfb :99, log: $WECHAT_LOG)..."
  xvfb-run -a -n 99 "${WECHAT_ENV[@]}" "$WECHAT_BIN" >"$WECHAT_LOG" 2>&1 &
  WECHAT_PID=$!
else
  echo "Starting WeChat with graphical display ($DISPLAY)..."
  "${WECHAT_ENV[@]}" "$WECHAT_BIN" >"$WECHAT_LOG" 2>&1 &
  WECHAT_PID=$!
fi

echo "Waiting for bridge socket at $SOCKET_PATH..."
READY=0
for ((i=1; i<=40; i++)); do
  if ! kill -0 "$WECHAT_PID" 2>/dev/null; then
    echo "WeChat exited unexpectedly. Last output from $WECHAT_LOG:" >&2
    tail -n 25 "$WECHAT_LOG" >&2 || true
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
  echo "Timed out waiting for bridge socket: $SOCKET_PATH" >&2
  exit 1
fi

if [[ "$USE_XVFB" -eq 1 ]]; then
  if ! python3 -c "import socket; s = socket.socket(); s.connect(('127.0.0.1', $HTTP_PORT)); s.close()" 2>/dev/null; then
    python3 -m http.server "$HTTP_PORT" --directory "$ROOT_DIR" >/dev/null 2>&1 &
    HTTP_PID=$!
  fi
  (
    sleep 2.5
    if command -v scrot >/dev/null 2>&1; then
      DISPLAY=:99 scrot -o "$ROOT_DIR/qrcode.png" 2>/dev/null || true
      if command -v zbarimg >/dev/null 2>&1 && command -v qrencode >/dev/null 2>&1 && [[ -f "$ROOT_DIR/qrcode.png" ]]; then
        qr_url=$(zbarimg -q --raw "$ROOT_DIR/qrcode.png" 2>/dev/null | head -n 1 || true)
        if [[ -n "$qr_url" ]]; then
          echo ""
          echo "==================== [控制台扫码登录] ===================="
          qrencode -t UTF8 "$qr_url"
          echo "=========================================================="
          echo "请直接用手机微信扫描上方二维码（务必勾选'自动登录'）"
          echo ""
          return
        fi
      fi
      if [[ -f "$ROOT_DIR/qrcode.png" ]]; then
        echo ""
        echo "=========================================================="
        echo "  [Scan QR Code Ready] WeChat QR code captured!"
        echo "  Open this link in your Windows browser:"
        echo "  👉 http://${HOST_IP}:${HTTP_PORT}/qrcode.png"
        echo "=========================================================="
        echo ""
      fi
    fi
  ) &
fi

echo "Bridge socket ready, launching bot daemon..."
export WX_BRIDGE_LIBRARY="$BRIDGE_LIB"
export WX_BRIDGE_PRELOADED=1
"$BOT_BIN" --root "$ROOT_DIR" --dll "$BRIDGE_LIB" &
BOT_PID=$!
wait "$BOT_PID"
