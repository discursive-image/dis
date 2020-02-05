// SPDX-FileCopyrightText: 2020 Jecoz
//
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/hypebeast/go-osc/osc"
)

var arg0 = filepath.Base(os.Args[0])
var logger = log.New(os.Stdout, "", log.LstdFlags)

func logf(format string, args ...interface{}) {
	logger.Printf(arg0+" * "+format, args...)
}

func errorf(format string, args ...interface{}) {
	logger.Printf(arg0+" error *"+format, args...)
}

func exitf(format string, args ...interface{}) {
	errorf(format, args...)
	os.Exit(1)
}

func openInput(path string) (io.ReadCloser, error) {
	if path == "-" {
		return os.Stdin, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("unable to open input file: %w", err)
	}
	return file, nil
}

// DI is a DiscoursiveImage.
type DI struct {
	Link    string
	Caption string
}

type StreamHandler struct {
	r       *csv.Reader
	clients struct {
		sync.Mutex
		m map[string]chan *DI
	}

	lastDI struct {
		sync.Mutex
		val *DI
	}
}

type RX struct {
	c     chan *DI
	close func()
}

func (h *StreamHandler) OpenRX() *RX {
	c := make(chan *DI, 1)

	// Inject last di processed to the new client.
	h.lastDI.Lock()
	if di := h.lastDI.val; di != nil {
		c <- di
	}
	h.lastDI.Unlock()

	h.clients.Lock()
	// generate a timestamp key inside the lock, so we're ensured to receive a unique one.
	key := fmt.Sprintf("%d", time.Now().UnixNano())
	if h.clients.m == nil {
		h.clients.m = make(map[string]chan *DI)
	}
	h.clients.m[key] = c
	h.clients.Unlock()

	return &RX{
		c: c,
		close: func() {
			defer close(c)

			h.clients.Lock()
			delete(h.clients.m, key)
			h.clients.Unlock()
		},
	}
}

// 00:00:00.400,00:00:00.540,all,https://i.ytimg.com/vi/HAfFfqiYLp0/maxresdefault.jpg
func decodeRecord(rec []string) (*DI, error) {
	if len(rec) < 4 {
		return nil, fmt.Errorf("unexpected record length %d, need at least 4", len(rec))
	}

	uri := rec[3]
	if _, err := url.ParseRequestURI(uri); err != nil {
		return nil, fmt.Errorf("unable to recognise url at position 3: %w", err)
	}

	// TODO: Parse caption
	return &DI{
		Link: uri,
	}, nil
}

func (h *StreamHandler) Run() {
	for {
		// Read next record from input.
		rec, err := h.r.Read()
		if err != nil && errors.Is(err, io.EOF) {
			os.Exit(0)
		}
		if err != nil {
			exitf("unable to read from input: %v", err)
		}

		// Decode it into a DI instance.
		di, err := decodeRecord(rec)
		if err != nil {
			errorf(err.Error())
			continue
		}

		// Send it to all clients.
		h.clients.Lock()
		for _, v := range h.clients.m {
			v <- di
		}
		h.clients.Unlock()

		// Save last di.
		h.lastDI.Lock()
		h.lastDI.val = di
		h.lastDI.Unlock()
	}
}

func (h *StreamHandler) HandleMessage(msg *osc.Message) {
	host, port, err := net.SplitHostPort(msg.Address)
	if err != nil {
		errorf("unable to make osc client: %v", err)
		return
	}
	p, err := strconv.Atoi(port)
	if err != nil {
		errorf("unable to convert port to int: %v", err)
		return
	}

	client := osc.NewClient(host, p)
	resp := osc.NewMessage("/di/next")
	rx := h.OpenRX()
	defer rx.close()

	for di := range rx.c {
		resp.ClearData()
		resp.Append(di.Link)
		resp.Append(di.Caption)
		if err := client.Send(resp); err != nil {
			errorf("unable to reply to %v: %v", host, err)
			return
		}
	}
}

func NewStreamHandler(in io.Reader) *StreamHandler {
	h := &StreamHandler{
		r: csv.NewReader(in),
	}
	go h.Run()
	return h
}

func main() {
	i := flag.String("i", "-", "Input file path. Use - for stdin.")
	p := flag.Int("p", 7745, "OSC server listening port.")
	flag.Parse()

	// Prepare input.
	in, err := openInput(*i)
	if err != nil {
		exitf(err.Error())
	}
	defer in.Close()

	h := NewStreamHandler(in)
	d := osc.NewStandardDispatcher()
	d.AddMsgHandler("/di/stream", h.HandleMessage)

	server := &osc.Server{
		Addr:       fmt.Sprintf("localhost:%d", *p),
		Dispatcher: d,
	}

	logf("OSC server listening on %d", *p)
	server.ListenAndServe()
}
