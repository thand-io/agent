package main

import (
	"bufio"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	var socketPath string
	flag.StringVar(&socketPath, "socket", `C:\temp\elevate-smoke.sock`, "unix socket path")
	flag.Parse()

	if err := os.MkdirAll(filepath.Dir(socketPath), 0o755); err != nil {
		exitf("create socket dir: %v", err)
	}
	_ = os.Remove(socketPath)

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		exitf("listen unix socket: %v", err)
	}
	defer func() {
		_ = ln.Close()
		_ = os.Remove(socketPath)
	}()

	fmt.Printf("listening on %s\n", socketPath)

	conn, err := ln.Accept()
	if err != nil {
		exitf("accept: %v", err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		exitf("read: %v", err)
	}
	line = strings.TrimRight(line, "\r\n")
	fmt.Printf("received: %q\n", line)

	if _, err := fmt.Fprintf(conn, "ack:%s\n", line); err != nil {
		exitf("write: %v", err)
	}
	fmt.Println("reply sent, exiting")
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
