// SPDX-FileCopyrightText: 2020 Jecoz
//
// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var arg0 = filepath.Base(os.Args[0])
var logger = log.New(os.Stdout, "", log.LstdFlags)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time to wait before force close on connection.
	closeGracePeriod = 10 * time.Second
)

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
	Link    string `json:"link"`
	Caption string `json:"caption"`
}

type StreamHandler struct {
	r       io.Reader
	clients struct {
		sync.Mutex
		m map[string]chan *DI
	}
	up websocket.Upgrader
	m  *mapset

	lastDI struct {
		sync.Mutex
		val *DI
	}
}

type diRx struct {
	c     chan *DI
	close func()
}

// OpenRx returns a new instance of a channel that is registered
// with the stream handler. Each time a new image is read, it is
// broadcasted to all registered channels.
// Remember to call `close` when done with it, to allow the handler
// to remove the channel from the list of registered clients.
func (h *StreamHandler) OpenRx() *diRx {
	c := make(chan *DI, 1)

	// Inject last di processed to the new client.
	h.lastDI.Lock()
	// Inside the lock we'll get unique time values.
	key := "stream:" + strconv.Itoa(int(time.Now().UnixNano()))
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

	return &diRx{
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

type mapset struct {
	cs int // column start
	ce int // column end
	cw int // column word
	cl int // column link
}

func (m *mapset) max() int {
	max := 0
	for _, v := range []int{m.cs, m.ce, m.cw, m.cl} {
		if v > max {
			max = v
		}
	}
	return max
}

// 00:00:00.400,00:00:00.540,all,https://i.ytimg.com/vi/HAfFfqiYLp0/maxresdefault.jpg
func decodeRecord(rec []string, m *mapset) (*DI, error) {
	if len(rec) < m.max() {
		return nil, fmt.Errorf("unexpected record length %d, need at least %d", len(rec), m.max())
	}

	uri := rec[m.cl]
	if _, err := url.ParseRequestURI(uri); err != nil {
		return nil, fmt.Errorf("unable to recognise url at position %d: %w", m.cl, err)
	}

	d, err := parseDuration(rec[m.cs])
	if err != nil {
		return nil, fmt.Errorf("unable to parse record start time: %w", err)
	}

	return &DI{
		Link:    uri,
		Caption: makeCaption(rec[m.cw], d),
	}, nil
}

// Run keeps on reading from `h`'s internal reader, providing its
// contents to the registered clients.
func (h *StreamHandler) Run() {
	logf("opening stream handler loop")
	defer logf("closing stream handler loop")
	r := csv.NewReader(h.r)

	for {
		// Read next record from input.
		rec, err := r.Read()
		if err != nil && errors.Is(err, io.EOF) {
			logf("input was closed (%v), exiting", err)
			os.Exit(0)
		}
		if err != nil {
			exitf("unable to read from input: %v", err)
		}
		logf("<--- %v", rec)

		// Decode it into a DI instance.
		di, err := decodeRecord(rec, h.m)
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

func wsError(ws *websocket.Conn, err error) {
	logf("websocket error: %v", err)
	ws.WriteMessage(websocket.TextMessage, []byte(err.Error()))
}

func (h *StreamHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	logf("connection from %v, %v", r.Host, r.URL)
	ws, err := h.up.Upgrade(w, r, nil)
	if err != nil {
		logf(err.Error())
		return
	}
	defer func() {
		logf("closing connection with %v", r.Host)
		ws.SetWriteDeadline(time.Now().Add(writeWait))
		ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		time.Sleep(closeGracePeriod)
		ws.Close()
	}()

	rx := h.OpenRx()
	defer rx.close()

	for di := range rx.c {
		if err := ws.WriteJSON(di); err != nil {
			wsError(ws, err)
			return
		}
	}
}

// NewStreamHandler returns a new http.Handler implementation that
// supports websockets.
func NewStreamHandler(in io.Reader, m *mapset) *StreamHandler {
	upgrader := websocket.Upgrader{
		ReadBufferSize:    4096,
		WriteBufferSize:   4096,
		EnableCompression: true,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	h := &StreamHandler{
		r:  bufio.NewReader(in),
		up: upgrader,
		m:  m,
	}
	go h.Run()
	return h
}

func main() {
	i := flag.String("i", "-", "Input file path. Use - for stdin.")
	p := flag.Int("p", 7745, "Server listening port.")
	cs := flag.Int("cs", 0, "Index of the column holding start information.")
	ce := flag.Int("ce", 1, "Index of the column holding end information.")
	cw := flag.Int("cw", 2, "Index of the column holding spoken word.")
	cl := flag.Int("cl", 5, "Index of the column holding image link.")
	flag.Parse()

	// Prepare input.
	logf("opening input from %v", *i)
	in, err := openInput(*i)
	if err != nil {
		exitf(err.Error())
	}
	defer in.Close()

	// Register the handler.
	h := NewStreamHandler(in, &mapset{
		cs: *cs,
		ce: *ce,
		cw: *cw,
		cl: *cl,
	})
	http.Handle("/di/stream", h)

	// Configure server.
	host := ":" + strconv.Itoa(*p)
	srv := &http.Server{
		Addr: host,
		// Good practice to set timeouts to avoid Slowloris attacks.
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
	}

	// Run our server in a goroutine so that it doesn't block.
	go func() {
		logf("server listening on %v", host)
		if err := srv.ListenAndServe(); err != nil {
			logf("server listener error: %v", err)
		}
	}()

	// Handle signals.
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt)

	sig := <-sigc

	// If in the meanwhile stdin is closed, the server will serve the last
	// content to the clients before exiting.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	logf("signal %v received, shutting down...", sig)
	srv.Shutdown(ctx)
}
