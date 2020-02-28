// SPDX-FileCopyrightText: 2020 Jecoz
//
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"time"

	"github.com/jecoz/dis/api"
)

var arg0 = filepath.Base(os.Args[0])
var logger = log.New(os.Stdout, "", log.LstdFlags)

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

func main() {
	i := flag.String("i", "-", "Input file path. Use - for stdin.")
	sd := flag.String("sd", "dimages-all", "Storage directory path - where images will be stored.")
	p := flag.Int("p", 7745, "Server listening port.")
	cs := flag.Int("cs", 1, "Index of the column holding start information.")
	ce := flag.Int("ce", 2, "Index of the column holding end information.")
	cw := flag.Int("cw", 3, "Index of the column holding spoken word.")
	cl := flag.Int("cl", 6, "Index of the column holding image link.")
	flag.Parse()

	// Prepare input.
	logf("opening input from %v", *i)
	in, err := openInput(*i)
	if err != nil {
		exitf(err.Error())
	}
	defer in.Close()

	// Register the file handler.
	fh := api.NewFileHandler(*sd)
	http.Handle("/di/images/", http.StripPrefix("/di/images/", fh))

	// Register stream handler.
	sh := api.NewStreamHandler(in, fh, api.NewMapSet(*cs, *ce, *cw, *cl))
	http.Handle("/di/stream", sh)

	// Configure server.
	host := ":" + strconv.Itoa(*p)
	srv := &http.Server{
		Addr:         host,
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

	select {
	case sig := <-sigc:
		logf("signal %v received, shutting down...", sig)
	case err := <-sh.Done:
		if err != nil {
			errorf("closing websocket server: %v", err)
		}
	}

	// If in the meanwhile stdin is closed, the server will serve the last
	// content to the clients before exiting.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	srv.Shutdown(ctx)
}
