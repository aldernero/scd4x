.PHONY: all build test lint clean

# Ignore parent go.work so this module builds/tests on its own.
export GOWORK := off

EXAMPLE_DIR := examples/monitor
EXAMPLE_BIN := $(EXAMPLE_DIR)/example_monitor

all: build test

build:
	go build -o $(EXAMPLE_BIN) ./$(EXAMPLE_DIR)

test:
	go test ./...

lint:
	golangci-lint run ./...

clean:
	rm -f $(EXAMPLE_BIN)
