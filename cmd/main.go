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
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	logger.Printf(arg0+" error * "+format, args...)
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

func (h *StreamHandler) OpenRX(host string, port int) *RX {
	key := fmt.Sprintf("%s:%d", host, port)
	c := make(chan *DI, 1)

	// Inject last di processed to the new client.
	h.lastDI.Lock()
	if di := h.lastDI.val; di != nil {
		c <- di
	}
	h.lastDI.Unlock()

	h.clients.Lock()
	if h.clients.m == nil {
		h.clients.m = make(map[string]chan *DI)
	}
	// Remove the client if was already there.
	if val, ok := h.clients.m[key]; ok {
		close(val)
		delete(h.clients.m, key)
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

// copy/pasted from git.keepinmind.info/subgendsk/sgenc/trrec.go
func parseDuration(raw string) (time.Duration, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 2 {
		return 0, fmt.Errorf("unable to split duration units from decimals")
	}
	units := strings.Split(parts[0], ":")

	// Validation
	if len(units) != 3 {
		return 0, fmt.Errorf("duration units should be in the form of hh:mm:ss, found %s", parts[0])
	}
	for i, v := range units {
		if len(v) != 2 {
			return 0, fmt.Errorf("invalid number of digits at position %d: found %d, but only 2 is allowed", i, len(v))
		}
	}
	if len(parts[1]) != 3 {
		return 0, fmt.Errorf("invalid number of millisecond digits: found %d, but only 3 is allowed", len(parts[1]))
	}

	h, _ := strconv.Atoi(units[0])
	m, _ := strconv.Atoi(units[1])
	s, _ := strconv.Atoi(units[2])
	ms, _ := strconv.Atoi(parts[1])

	// Validation
	if m > 59 {
		return 0, fmt.Errorf("invalid minutes field: must be less than 59")
	}
	if s > 59 {
		return 0, fmt.Errorf("invalid seconds field: must be less than 59")
	}

	d := time.Duration(0)
	d += time.Duration(h) * time.Hour
	d += time.Duration(m) * time.Minute
	d += time.Duration(s) * time.Second
	d += time.Duration(ms) * time.Millisecond

	return d, nil
}

func makeCaption(w string, d time.Duration) string {
	return fmt.Sprintf("\"%s\", heard after %v", w, d)
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

	d, err := parseDuration(rec[0])
	if err != nil {
		return nil, fmt.Errorf("unable to parse record start time: %w", err)
	}

	return &DI{
		Link:    uri,
		Caption: makeCaption(rec[2], d),
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

func (h *StreamHandler) HandleStream(msg *osc.Message) {
	// Find and validate type tags.
	tt, err := msg.TypeTags()
	if err != nil {
		errorf("unable to decode message: %v", err)
		return
	}
	if tt != ",si" {
		errorf("unexpected type tags field %v, wanted ,si", tt)
		return
	}

	// Retrieve host.
	host, ok := msg.Arguments[0].(string)
	if !ok {
		errorf("unable to parse client stream host")
		return
	}

	// Retrieve port.
	port, ok := msg.Arguments[1].(int32)
	if !ok {
		errorf("unable to parse client stream port")
		return
	}

	client := osc.NewClient(host, int(port))
	resp := osc.NewMessage("/di/next")
	rx := h.OpenRX(host, int(port))
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
	d.AddMsgHandler("/di/stream", h.HandleStream)

	server := &osc.Server{
		Addr:       fmt.Sprintf("localhost:%d", *p),
		Dispatcher: d,
	}

	logf("OSC server listening on %d", *p)
	server.ListenAndServe()
}
