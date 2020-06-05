# SPDX-FileCopyrightText: 2020 Jecoz
#
# SPDX-License-Identifier: MIT

export GO111MODULE=on

all: dis examples

dis:
	go build -v -o bin/$@ ./cmd/$@
examples: ratereader echoclient replayreader
ratereader:
	go build -v -o bin/$@ ./cmd/$@
echoclient:
	go build -v -o bin/$@ ./cmd/$@
replayreader:
	go build -v -o bin/$@ ./cmd/$@

format:
	go fmt ./...
clean:
	rm -rf bin

