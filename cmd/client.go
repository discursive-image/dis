// SPDX-FileCopyrightText: 2020 Jecoz
//
// SPDX-License-Identifier: MIT

package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/gorilla/websocket"
)

var arg0 = filepath.Base(os.Args[0])

func logf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, arg0+" * "+format+"\n", args...)
}

func errorf(format string, args ...interface{}) {
	logf("error: "+format, args...)
}

func exitf(format string, args ...interface{}) {
	errorf(format, args...)
	os.Exit(1)
}

func main() {
	host := flag.String("h", "localhost:7745", "Websocket address to connect to")
	flag.Parse()

	u := url.URL{Scheme: "ws", Host: *host, Path: "/di/stream"}
	logf("connecting to %v", u.String())

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		exitf("dial: %v", err)
	}
	defer c.Close()

	// Handle signals.
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				errorf("read: %v", err)
				return
			}
			logf("recv: %s", message)
		}
	}()

	for {
		select {
		case <-done:
			return
		case <-interrupt:
			logf("interrupt signal received")

			// Cleanly close the connection by sending a close message and then
			// waiting (with timeout) for the server to close the connection.
			err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				logf("write close:", err)
				return
			}
			select {
			case <-done:
			case <-time.After(time.Second):
			}
			return
		}
	}
}
