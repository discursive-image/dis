package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/discursive-image/dis/api"
	"github.com/discursive-image/dis/api/ws"
	"github.com/hypebeast/go-osc/osc"
)

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
	sd := flag.String("sd", "images", "Storage directory path - where images will be stored.")
	p := flag.Int("p", 7745, "Server listening port.")
	oscp := flag.Int("oscp", 5498, "OSC server listening port.")
	osch := flag.String("osch", "localhost", "OSC server host.")
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

	srv := &ws.Server{
		Fh:      api.NewFileHandler(*sd),
		Port:    *p,
		Mapping: ws.NewMapping(*cs, *ce, *cw, *cl),
		Osc:     osc.NewClient(*osch, *oscp),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals.
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt)

	go func() {
		select {
		case sig := <-sigc:
			logf("signal %v received, shutting down...", sig)
			cancel()
		}
	}()

	srv.Run(ctx, in)
}
