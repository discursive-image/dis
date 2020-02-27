# SPDX-FileCopyrightText: 2020 Jecoz
#
# SPDX-License-Identifier: MIT

all: dis
dis: cmd/main.go
	go build -v -o bin/$@ $^

examples: ratereader client replayreader
ratereader:
	go build -v -o bin/$@ cmd/ratereader.go
client:
	go build -v -o bin/$@ cmd/client.go
replayreader:
	go build -v -o bin/$@ cmd/replayreader.go

format:
	go fmt ./...

