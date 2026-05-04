.PHONY: build run tidy test clean wasm wasm-serve

PORT       ?= 8765
WEB        := web
WASM_OUT   := $(WEB)/sim.wasm
EXEC_OUT   := $(WEB)/wasm_exec.js
FOXPRO_OUT := $(WEB)/foxpro.js
GOROOT     := $(shell go env GOROOT)
EXEC_SRC   := $(firstword $(wildcard $(GOROOT)/lib/wasm/wasm_exec.js $(GOROOT)/misc/wasm/wasm_exec.js))
# foxpro-go ships a shared canvas-renderer + input bridge in
# wasm/foxpro.js. Resolve its path from the module cache so we
# always copy the version matching go.mod's pinned tag.
FOXPRO_SRC := $(shell go list -m -f '{{.Dir}}' github.com/carledwards/foxpro-go)/wasm/foxpro.js

build:
	go build -o bin/6502-sim ./cmd/6502-sim

run: build
	./bin/6502-sim

# wasm — compile the browser build into web/ and copy the JS shim from
# the active Go toolchain. Re-run after editing any Go file; `go build`
# does its own dependency check so this is fast on no-op builds.
wasm:
	@mkdir -p $(WEB)
	GOOS=js GOARCH=wasm go build -o $(WASM_OUT) ./cmd/6502-wasm
	@if [ -z "$(EXEC_SRC)" ]; then \
	  echo "ERROR: wasm_exec.js not found under $(GOROOT)/{lib,misc}/wasm/"; \
	  exit 1; \
	fi
	@cp $(EXEC_SRC) $(EXEC_OUT)
	@if [ ! -f "$(FOXPRO_SRC)" ]; then \
	  echo "ERROR: foxpro.js not found at $(FOXPRO_SRC) — run 'go mod tidy' first"; \
	  exit 1; \
	fi
	@cp $(FOXPRO_SRC) $(FOXPRO_OUT)
	@ls -lh $(WASM_OUT) | awk '{print "  built " $$NF " (" $$5 ")"}'

# wasm-serve — local static server on PORT (default 8765). Use after
# `make wasm`. Open http://localhost:$(PORT)/ once it starts.
wasm-serve:
	@echo "serving $(WEB)/ at http://localhost:$(PORT)/"
	@cd $(WEB) && python3 -m http.server $(PORT)

tidy:
	go mod tidy

test:
	go test ./...

clean:
	rm -rf bin $(WASM_OUT) $(EXEC_OUT) $(FOXPRO_OUT)
