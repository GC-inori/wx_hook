#!/usr/bin/env python3
"""Runtime-only smoke test for locally built Linux artifacts; no WeChat changes."""
import argparse
import base64
import ctypes
import http.client
import json
import os
import secrets
import socket
import subprocess
import tempfile
import time
from pathlib import Path


ACTIONS = {
    "sendText": 400, "sendText2": 400, "sendXml": 400, "sendImage": 400,
    "sendFile": 400, "sendGif": 400, "sendCard": 400, "sendShareUrl": 400,
    "sendApplet": 400, "sendReferText": 400, "revokeMyMsg": 501,
    "getSelfInfo": 401, "getMemberNick": 400, "getFriendList": 501,
    "getGroupList": 501, "getMemberList": 501, "getPublicList": 501,
    "getLabelList": 501, "queryObj": 501, "queryGroup": 501,
    "createGroup": 501, "addMembers": 501, "inviteMembers": 501,
    "delMembers": 501, "quitGroup": 501, "editSelfMemberNick": 501,
    "agreeFriendReq": 501, "delFriend": 501, "editObjRemark": 501,
    "confirmTrans": 501, "returnTrans": 501, "setReadStatus": 501,
}


def require(condition, detail):
    if not condition:
        raise RuntimeError(detail)


def run(root):
    with tempfile.TemporaryDirectory(prefix="wx-gateway-smoke-") as work:
        bridge_socket = str(Path(work) / "bridge.sock")
        os.environ["WX_BRIDGE_SOCKET"] = bridge_socket
        os.environ["WX_BRIDGE_LOG_DIR"] = work
        library = ctypes.CDLL(str(root / "librust_bridge.so"))
        library.WxStartBridge.argtypes = []
        library.WxStartBridge.restype = ctypes.c_int
        require(library.WxStartBridge() == 0, "bridge startup failed")
        deadline = time.monotonic() + 5
        while not Path(bridge_socket).is_socket():
            require(time.monotonic() < deadline, "bridge socket startup timed out")
            time.sleep(0.02)
        with socket.socket(socket.AF_UNIX) as native:
            native.settimeout(3)
            native.connect(bridge_socket)
            with native.makefile("rb") as reader:
                for request_id, (action, code) in enumerate(
                    list(ACTIONS.items()) + [("checkWeChat", 200), ("get_account", 401), ("unknown_action", 404)], 1
                ):
                    native.sendall(json.dumps({"id": request_id, "action": action, "data": {}}).encode() + b"\n")
                    response = json.loads(reader.readline(3 * 1024 * 1024 + 1))
                    require(response.get("id") == request_id, f"{action}: lost response correlation")
                    require(response.get("code") == code, f"{action}: {response}")
                    require(all(key in response for key in ("code", "msg", "status", "error", "result", "data")), f"{action}: incomplete response")
                    require(response["data"] == response["result"], f"{action}: result/data mismatch")
        print("Native socket: all 32 public actions, ping/account aliases, response fields, and unknown-action handling passed.")

        with socket.socket() as reservation:
            reservation.bind(("127.0.0.1", 0))
            port = reservation.getsockname()[1]
        token = secrets.token_urlsafe(24)
        environment = dict(os.environ, WX_GATEWAY_TOKEN=token)
        with open(Path(work) / "gateway.log", "w+b") as output:
            process = subprocess.Popen(
                [str(root / "wxbot"), "-root", str(root), "-listen", f"127.0.0.1:{port}"],
                env=environment, stdout=output, stderr=subprocess.STDOUT, stdin=subprocess.DEVNULL,
            )
            try:
                def request(method, path, body=None, authorized=True):
                    connection = http.client.HTTPConnection("127.0.0.1", port, timeout=3)
                    try:
                        headers = {"Content-Type": "application/json"}
                        if authorized:
                            headers["Authorization"] = "Bearer " + token
                        connection.request(method, path, body, headers)
                        response = connection.getresponse()
                        return response.status, json.loads(response.read())
                    finally:
                        connection.close()

                deadline = time.monotonic() + 5
                while True:
                    require(process.poll() is None, "gateway exited before readiness")
                    try:
                        status, data = request("GET", "/healthz")
                        if status == 200:
                            break
                    except OSError:
                        pass
                    require(time.monotonic() < deadline, "gateway readiness timed out")
                    time.sleep(0.05)
                require(data["result"]["connected"] is True, "gateway not connected to isolated native bridge")
                require(request("GET", "/healthz", authorized=False)[0] == 401, "gateway authentication check failed")
                for action, expected in ACTIONS.items():
                    status, response = request("POST", "/api", json.dumps({"action": action, "params": {}}))
                    expected_retcode = 1400 if expected == 400 else 1401 if expected == 401 else 1405 if expected == 501 else 0
                    require(status == 200 and response.get("retcode") == expected_retcode, f"HTTP {action}: {status} {response}")
                with socket.create_connection(("127.0.0.1", port), timeout=3) as websocket:
                    key = base64.b64encode(secrets.token_bytes(16)).decode()
                    websocket.sendall((f"GET /ws HTTP/1.1\r\nHost: 127.0.0.1:{port}\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: {key}\r\nAuthorization: Bearer {token}\r\n\r\n").encode())
                    header = b""
                    while b"\r\n\r\n" not in header:
                        chunk = websocket.recv(4096)
                        require(bool(chunk) and len(header) < 16384, "incomplete WebSocket handshake")
                        header += chunk
                    require(header.startswith(b"HTTP/1.1 101 "), f"WebSocket upgrade failed: {header!r}")
                print("Gateway: health, authentication, all 32 HTTP actions (OneBot 11), and WebSocket upgrade passed.")
            except Exception:
                output.flush()
                output.seek(0)
                print(output.read().decode(errors="replace"))
                raise
            finally:
                if process.poll() is None:
                    process.terminate()
                    try:
                        process.wait(timeout=15)
                    except subprocess.TimeoutExpired:
                        process.kill()
                        process.wait(timeout=3)
                require(process.returncode == 0, f"gateway did not shut down cleanly: {process.returncode}")
        print("Gateway SIGTERM shutdown passed. Existing WeChat and gateway processes were not restarted.")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=Path("/opt/wx"))
    run(parser.parse_args().root.resolve())
