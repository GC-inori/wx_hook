# Linux preload bridge

This module provides the Linux `LD_PRELOAD` implementation of `librust_bridge.so` for 64-bit Linux builds.

## Build and load

Build on Debian 12.5 (or a compatible x86-64 Linux host):

```sh
cargo build --target x86_64-unknown-linux-gnu --release
```

The shared library is written to `target/x86_64-unknown-linux-gnu/release/librust_bridge.so`. Load it before starting the WeChat process:

```sh
LD_PRELOAD=/absolute/path/to/librust_bridge.so /path/to/wechat
```

The bridge listens at `/tmp/wx_bridge.sock` by default. Set `WX_BRIDGE_SOCKET` before process startup to use another socket path. The socket is created with mode `0600`; stale socket files are removed only when a connection attempt reports that no listener is present. A non-socket file at the requested path is never replaced.

## IPC wire format

IPC uses newline-delimited JSON over a Unix domain stream socket. Each request and response occupies one JSON line. Event messages and command responses share the connection, so clients should distinguish event lines (which contain `event`) from responses (which contain `id` and `status`).

Ping request:

```json
{"id":1,"action":"ping"}
```

Raw MMMojo write request:

```json
{"id":2,"action":"send_raw","data":{"method":7,"sync":false,"request_id":42,"hex":"010203a0"}}
```

`send_raw` requires integer `method`, boolean `sync`, unsigned 32-bit `request_id`, and an even-length hex string `hex`. Hex may contain ASCII whitespace or an optional leading `0x`. Payloads are limited to 1 MiB. A successful send response has `status: 0` and `data: {"sent":true,"bytes":4}`; a false native send result uses `status: 2`.

Incoming packets are broadcast as lines like:

```json
{"event":"mmmojo_read","data":{"callback_type":1,"request_id":42,"request_data_size":3,"attach_data_size":0,"request_hex":"010203","attach_hex":"","request_preview":"...","attach_preview":"","truncated":false}}
```

Outgoing packets use `event: "mmmojo_write"` and include the method, sync flag, request ID, byte count, hex data, and a UTF-8 preview when their write buffer was observed through the proxy's `CreateMMMojoWriteInfo` and `GetMMMojoWriteInfoRequest` exports. Arbitrary pre-existing write-info objects may not have recoverable payload size because the supplied ABI has no size query; such events report `payload_available: false` rather than guessing a buffer length. Hex capture is capped at 64 KiB and raw capture at 1 MiB; `truncated` indicates that at least one capture representation was clipped.

## Hook behavior

The Linux module exports proxies for callback registration, read request/attachment access, write-info creation/buffer access, and write submission. `dlsym(RTLD_NEXT, ...)` resolves each original symbol once. Read-push and read-pull registrations are forwarded with small callback trampolines that capture the packet, then call the original callback with its original user-data pointer. Other callback types are forwarded unchanged. The most recently registered environment pointer is retained for `send_raw`.
