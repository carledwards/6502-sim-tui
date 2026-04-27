.PHONY: build run tidy test clean

build:
	go build -o bin/6502-sim ./cmd/6502-sim

run: build
	./bin/6502-sim

tidy:
	go mod tidy

test:
	go test ./...

clean:
	rm -rf bin
