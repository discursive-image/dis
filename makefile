# SPDX-FileCopyrightText: 2020 Jecoz
#
# SPDX-License-Identifier: MIT

all: dis examples
dis: cmd/main.go
	go build -v -o bin/$@ $^
examples: ratereader
ratereader:
	go build -v -o bin/$@ cmd/ratereader.go
format:
	go fmt ./...

