# SPDX-FileCopyrightText: 2020 Jecoz
#
# SPDX-License-Identifier: MIT

dis: cmd/main.go
	go build -v -o bin/$@ $^
examples: osc-client ratereader
osc-client:
	go build -v -o bin/$@ examples/client.go
ratereader:
	go build -v -o bin/$@ examples/ratereader.go
test:
	go test ./...
format:
	go fmt ./...

