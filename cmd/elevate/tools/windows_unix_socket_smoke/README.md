# Windows Unix Socket Smoke Test

One-time pair of tools to verify Unix socket support on Windows with Go `net`.

## Run

In terminal 1:

```powershell
cd cmd/elevate
go run ./tools/windows_unix_socket_smoke/server -socket "C:\temp\elevate-smoke.sock"
```

In terminal 2:

```powershell
cd cmd/elevate
go run ./tools/windows_unix_socket_smoke/client -socket "C:\temp\elevate-smoke.sock" -message "hello"
```

Expected:
- server prints `received: "hello"` and exits
- client prints `reply: "ack:hello"`

