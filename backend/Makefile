.PHONY: run-server run build clean ask upload

# Run the server with hot-reloading using gow
run-server:
	gow run ./cmd/devask serve

# Standard go run without hot-reloading
run:
	go run ./cmd/devask serve

# Build the application
build:
	go build -o devask.exe ./cmd/devask/

# Clean build artifacts
clean:
	rm -rf ./tmp devask.exe

# Quick smoke test: ask a question (requires server running)
# Usage: make ask Q="How does our auth system work?"
ask:
	go run ./cmd/devask ask $(Q)

# Quick upload: upload a file (requires server running)
# Usage: make upload F="path/to/your/doc.md"
upload:
	go run ./cmd/devask upload $(F)
