// SPDX-FileCopyrightText: 2020 Jecoz
//
// SPDX-License-Identifier: MIT

package api

import (
	"bufio"
	"crypto/md5"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
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
	logf(" error: "+format, args...)
}

func exitf(format string, args ...interface{}) {
	errorf(format, args...)
	os.Exit(1)
}

// DI is a DiscoursiveImage.
type DI struct {
	Link     string `json:"link"`
	Word     string `json:"word"`
	FileName string `json:"file_name"`
}

type mapset struct {
	cs int // column start
	ce int // column end
	cw int // column word
	cl int // column link
}

func NewMapSet(cs, ce, cw, cl int) *mapset {
	return &mapset{
		cs: cs,
		ce: ce,
		cw: cw,
		cl: cl,
	}
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

type FileSystem interface {
	Exists(string) bool
	Create(string) (*os.File, error)
}

type StreamHandler struct {
	r       io.Reader
	sd      string // storage directory path.
	clients struct {
		sync.Mutex
		m map[string]chan *DI
	}
	up   websocket.Upgrader
	m    *mapset
	Done chan error
	fs   FileSystem

	lastDI struct {
		sync.Mutex
		val *DI
	}
}

type diRx struct {
	c     chan *DI
	close func()
}

// NewStreamHandler returns a new http.Handler implementation that
// supports websockets.
func NewStreamHandler(in io.Reader, fs FileSystem, m *mapset) *StreamHandler {
	upgrader := websocket.Upgrader{
		ReadBufferSize:    4096,
		WriteBufferSize:   4096,
		EnableCompression: true,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	h := &StreamHandler{
		fs:   fs,
		r:    bufio.NewReader(in),
		up:   upgrader,
		m:    m,
		Done: make(chan error, 1),
	}
	go h.Run()
	return h
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

// 00:00:00.000,00:00:00.400,00:00:00.540,all,https://i.ytimg.com/vi/HAfFfqiYLp0/maxresdefault.jpg
func decodeRecord(rec []string, m *mapset) (*DI, error) {
	if len(rec) < m.max() {
		return nil, fmt.Errorf("unexpected record length %d, need at least %d", len(rec), m.max())
	}

	uri := rec[m.cl]
	u, err := url.ParseRequestURI(uri)
	if err != nil {
		return nil, fmt.Errorf("unable to recognise url at position %d: %w", m.cl, err)
	}

	word := rec[m.cw]

	h := md5.New()
	io.WriteString(h, uri)
	fn := fmt.Sprintf("%s-%x%s", word, h.Sum(nil), filepath.Ext(u.Path))

	return &DI{
		FileName: url.PathEscape(fn),
		Link:     uri,
		Word:     word,
	}, nil
}

func (h *StreamHandler) handleRecord(rec []string) (*DI, error) {
	di, err := decodeRecord(rec, h.m)
	if err != nil {
		return nil, err
	}

	// If the file is already there, do not download again.
	if h.fs.Exists(di.FileName) {
		return di, nil
	}

	// Otherwise download it.
	f, err := h.fs.Create(di.FileName)
	if err != nil {
		return nil, fmt.Errorf("unable to prepare file for storing image: %w", err)
	}
	defer f.Close()

	logf("downloading image for: %v", di.FileName)

	resp, err := http.Get(di.Link)
	if err != nil {
		return nil, fmt.Errorf("unable to download image: %w", err)
	}
	defer resp.Body.Close()
	if _, err = io.Copy(f, resp.Body); err != nil {
		return nil, fmt.Errorf("unable to store image: %w", err)
	}

	return di, nil
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
			h.Done <- nil
		}
		if err != nil {
			h.Done <- fmt.Errorf("unable to read from input: %v", err)
			return
		}
		di, err := h.handleRecord(rec)
		if err != nil {
			errorf(err.Error())
			continue
		}

		logf("---> %v", di.FileName)

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
	logf("connection from %v, %v", r.RemoteAddr, r.URL)
	ws, err := h.up.Upgrade(w, r, nil)
	if err != nil {
		logf(err.Error())
		return
	}
	defer func() {
		logf("closing connection with %v", r.RemoteAddr)
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
