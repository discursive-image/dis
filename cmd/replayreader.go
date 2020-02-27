// SPDX-FileCopyrightText: 2020 Jecoz
//
// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var arg0 = filepath.Base(os.Args[0])

func errorf(format string, args ...interface{}) {
	fmt.Printf(arg0+" error * "+format, args...)
}

func exitf(format string, args ...interface{}) {
	errorf(format, args...)
	os.Exit(1)
}

// copy/pasted from https://git.keepinmind.info/subgensdk/sgenc,
// Licensed under MIT, still not open source.
// TODO: import the library as soon as it is available.
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

func main() {
	r := csv.NewReader(bufio.NewReader(os.Stdin))
	r.ReuseRecord = true
	w := csv.NewWriter(bufio.NewWriter(os.Stdout))
	start := time.Now()

	for {
		rec, err := r.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			exitf("unable to read records: %v", err)
		}
		d, err := parseDuration(rec[0])
		if err != nil {
			errorf("unable to parse record written at duration: %v", err)
			continue
		}

		wait := start.Add(d).Sub(time.Now())
		<-time.After(wait)
		if err = w.Write(rec); err != nil {
			exitf("unable to write record: %v", err)
		}
		w.Flush()
	}
}
