package main

import (
	"bufio"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

func main() {
	var (
		socketPath string
		message    string
		timeout    time.Duration
	)

	flag.StringVar(&socketPath, "socket", `C:\temp\elevate-smoke.sock`, "unix socket path")
	flag.StringVar(&message, "message", "ping", "line payload to send")
	flag.DurationVar(&timeout, "timeout", 5*time.Second, "dial/read/write timeout")
	flag.Parse()

	conn, err := net.DialTimeout("unix", socketPath, timeout)
	if err != nil {
		exitf("dial: %v", err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		exitf("set deadline: %v", err)
	}

	if _, err := fmt.Fprintf(conn, "%s\n", message); err != nil {
		exitf("write: %v", err)
	}

	reader := bufio.NewReader(conn)
	reply, err := reader.ReadString('\n')
	if err != nil {
		exitf("read: %v", err)
	}

	fmt.Printf("reply: %q\n", strings.TrimRight(reply, "\r\n"))
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
