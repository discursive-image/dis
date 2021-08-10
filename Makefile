export BINDIR ?= $(abspath bin)

PREFIX :=
SRC := cmd/*/*.go api/*.go api/*/*.go
TARGETS := dis echoclient ratereader replayreader

BINNAMES := $(addprefix $(PREFIX), $(TARGETS))
BINS := $(addprefix $(BINDIR)/, $(BINNAMES))

all: $(BINS)
test: $(SRC); go test ./...

$(BINDIR)/$(PREFIX)%: $(SRC); go build -o $@ cmd/$*/main.go
$(BINS): | $(BINDIR)
$(BINDIR):; mkdir -p $(BINDIR)

clean:; rm -rf $(BINDIR)/$(PREFIX)*
