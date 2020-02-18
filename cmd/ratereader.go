// SPDX-FileCopyrightText: 2020 Jecoz
//
// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
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

func main() {
	i := flag.Int("i", 1000, "Interval in milliseconds between each read.")
	flag.Parse()

	scanner := bufio.NewScanner(os.Stdin)
	writer := bufio.NewWriter(os.Stdout)
	for scanner.Scan() {
		<-time.After(time.Millisecond * time.Duration(*i))
		if _, err := writer.WriteString(scanner.Text() + "\n"); err != nil {
			exitf(err.Error())
		}
		writer.Flush()
	}

	if err := scanner.Err(); err != nil {
		exitf(err.Error())
	}
}
