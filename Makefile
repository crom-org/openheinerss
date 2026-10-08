.PHONY: all build test clean install run-mock doctor

BINARY_NAME=openheinerss
BINARY_PATH=bin/$(BINARY_NAME)
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null || echo desconhecido)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS=-X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.Date=$(DATE)

all: test build

build:
	mkdir -p bin
	go build -ldflags '$(LDFLAGS)' -o $(BINARY_PATH) ./cmd/openheinerss

test:
	go test -v ./...

doctor: build
	./$(BINARY_PATH) doctor

run-mock: build
	./$(BINARY_PATH) run --harness mock "Analise o projeto"

install: build
	mkdir -p $(HOME)/.local/bin
	cp $(BINARY_PATH) $(HOME)/.local/bin/$(BINARY_NAME)
	@echo "Instalado com sucesso em $(HOME)/.local/bin/$(BINARY_NAME)"

clean:
	rm -f $(BINARY_PATH)
	rm -rf dist/
