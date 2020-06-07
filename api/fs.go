// SPDX-FileCopyrightText: 2020 Jecoz
//
// SPDX-License-Identifier: MIT

package api

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
)

var arg0 = filepath.Base(os.Args[0])

func logf(format string, args ...interface{}) {
	log.Printf(arg0+" * "+format, args...)
}

type FileHandler struct {
	fs  http.Handler
	dir string
}

func NewFileHandler(d string) *FileHandler {
	return &FileHandler{
		fs:  http.FileServer(http.Dir(d)),
		dir: d,
	}
}

func (f *FileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	logf("file request from %v: %v", r.RemoteAddr, r.URL.Path)
	f.fs.ServeHTTP(w, r)
}

func (f *FileHandler) Exists(path string) bool {
	_, err := os.Stat(filepath.Join(f.dir, path))
	return err == nil
}

func (f *FileHandler) Create(path string) (*os.File, error) {
	fn := filepath.Join(f.dir, path)
	if err := os.MkdirAll(filepath.Dir(fn), os.ModePerm); err != nil {
		return nil, err
	}
	return os.Create(fn)
}
