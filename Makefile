.PHONY: all build test clean install run-mock doctor

BINARY_NAME=openheinerss

all: test build

build:
	go build -o $(BINARY_NAME) ./cmd/openheinerss

test:
	go test -v ./...

doctor: build
	./$(BINARY_NAME) doctor

run-mock: build
	./$(BINARY_NAME) run --harness mock "Analise o projeto"

install: build
	mkdir -p $(HOME)/.local/bin
	cp $(BINARY_NAME) $(HOME)/.local/bin/$(BINARY_NAME)
	@echo "Instalado com sucesso em $(HOME)/.local/bin/$(BINARY_NAME)"

clean:
	rm -f $(BINARY_NAME)
	rm -rf dist/
