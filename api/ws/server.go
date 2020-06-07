// SPDX-FileCopyrightText: 2020 Jecoz
//
// SPDX-License-Identifier: MIT

package ws

import (
	"context"
	"crypto/md5"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/discursive-image/dis/api"
	"github.com/gorilla/websocket"
	"github.com/hypebeast/go-osc/osc"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type FileSystem interface {
	Exists(string) bool
	Create(string) (*os.File, error)
}

var arg0 = filepath.Base(os.Args[0])

func logf(format string, args ...interface{}) {
	log.Printf(arg0+" * "+format, args...)
}

func errorf(format string, args ...interface{}) {
	logf(" error: "+format, args...)
}

func exitf(format string, args ...interface{}) {
	errorf(format, args...)
	os.Exit(1)
}

type mapping struct {
	cs int // column start
	ce int // column end
	cw int // column word
	cl int // column link
}

func NewMapping(cs, ce, cw, cl int) *mapping {
	return &mapping{
		cs: cs,
		ce: ce,
		cw: cw,
		cl: cl,
	}
}

func (m *mapping) max() int {
	max := 0
	for _, v := range []int{m.cs, m.ce, m.cw, m.cl} {
		if v > max {
			max = v
		}
	}
	return max
}

type DI struct {
	StartAt  time.Duration `json:"start_at"`
	EndAt    time.Duration `json:"end_at"`
	Link     string        `json:"link"`
	Word     string        `json:"word"`
	FileName string        `json:"file_name"`
}

type Server struct {
	Fh      *api.FileHandler
	Osc     *osc.Client
	Port    int
	Done    chan error
	Mapping *mapping
	hub     *Hub
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	logf("connection from %v, %v", r.RemoteAddr, r.URL)
	s.ServeWs(w, r)
}

func ParseDuration(raw string) (time.Duration, error) {
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

// 00:00:00.000,00:00:00.400,00:00:00.540,all,https://i.ytimg.com/vi/HAfFfqiYLp0/maxresdefault.jpg
func decodeDI(m *mapping, rec []string) (*DI, error) {
	if len(rec) < m.max() {
		return nil, fmt.Errorf("unexpected record length %d, need at least %d", len(rec), m.max())
	}

	uri := rec[m.cl]
	u, err := url.ParseRequestURI(uri)
	if err != nil {
		return nil, fmt.Errorf("unable to recognise url at position %d: %w", m.cl, err)
	}

	word := rec[m.cw]

	start, err := ParseDuration(rec[m.cs])
	if err != nil {
		return nil, fmt.Errorf("unable to parse start duration: %w", err)
	}
	end, err := ParseDuration(rec[m.ce])
	if err != nil {
		return nil, fmt.Errorf("unable to parse end duration: %w", err)
	}

	h := md5.New()
	io.WriteString(h, uri)
	io.WriteString(h, word)
	fn := fmt.Sprintf("%x%s", h.Sum(nil), filepath.Ext(u.Path))

	return &DI{
		FileName: fn,
		Link:     uri,
		Word:     word,
		StartAt:  start,
		EndAt:    end,
	}, nil
}

func downloadImage(fs FileSystem, di *DI) error {
	// If the file is already there, do not download again.
	if fs.Exists(di.FileName) {
		return nil
	}

	// Otherwise download it.
	f, err := fs.Create(di.FileName)
	if err != nil {
		return fmt.Errorf("unable to prepare file for storing image: %w", err)
	}
	defer f.Close()

	logf("downloading image for: %v", di.FileName)

	resp, err := http.Get(di.Link)
	if err != nil {
		return fmt.Errorf("unable to download image: %w", err)
	}
	defer resp.Body.Close()
	if _, err = io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("unable to store image: %w", err)
	}

	return nil
}

func (s *Server) read(ctx context.Context, in io.Reader) {
	r := csv.NewReader(in)
	for {
		// Read next record from input.
		rec, err := r.Read()
		if err != nil && errors.Is(err, io.EOF) {
			logf("input was closed (%v), exiting", err)
			s.Done <- nil
			return
		}
		if err != nil {
			err = fmt.Errorf("unable to read from input: %v", err)
			errorf(err.Error())
			s.Done <- err
			return
		}

		di, err := decodeDI(s.Mapping, rec)
		if err != nil {
			errorf(err.Error())
			continue
		}
		if err = downloadImage(s.Fh, di); err != nil {
			errorf(err.Error())
			continue
		}

		logf("---> %v", di.FileName)
		s.hub.broadcast <- di
	}
}

func (s *Server) Run(ctx context.Context, in io.Reader) {
	logf("opening stream handler loop")
	defer logf("closing stream handler loop")

	s.hub = newHub()
	s.Done = make(chan error, 1)
	mux := http.NewServeMux()
	mux.Handle("/di/images/", http.StripPrefix("/di/images/", s.Fh))
	mux.Handle("/di/stream", s)

	host := net.JoinHostPort("", strconv.Itoa(s.Port))
	srv := &http.Server{
		Addr:         host,
		Handler:      mux,
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go s.hub.run()
	go s.read(ctx, in)

	go func() {
		logf("server listening on %v", host)
		if err := srv.ListenAndServe(); err != nil {
			logf("server listener error: %v", err)
			cancel()
		}
	}()

	select {
	case err := <-s.Done:
		if err != nil {
			errorf("closing websocker server: %v", err)
		}
	case <-ctx.Done():
		errorf("context invalidated: %v", ctx.Err())
	}

	// If in the meanwhile stdin is closed, the server will serve the last
	// content to the clients before exiting.
	ctx, cancel = context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	srv.Shutdown(ctx)
}

func (s *Server) ServeWs(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		errorf(err.Error())
		return
	}
	client := &Client{
		Addr: r.RemoteAddr,
		hub:  s.hub,
		conn: conn,
		send: make(chan *DI, 50),
		osc:  s.Osc,
	}
	client.hub.register <- client

	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines.
	go client.forwardMessages()
	go client.readMessages()
}
