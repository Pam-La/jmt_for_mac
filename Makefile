GOEXPERIMENT ?= arenas
GOOS ?= darwin
GOARCH ?= arm64

export GOEXPERIMENT
export GOOS
export GOARCH

.PHONY: test race bench bench-hash bench-jmt

test:
	go test ./...

race:
	go test -race ./...

bench:
	go test -run=^$$ -bench=. -benchmem ./...

bench-hash:
	go test -run=^$$ -bench=Blake3 -benchmem ./internal/hash

bench-jmt:
	go test -run=^$$ -bench=JMT -benchmem ./internal/jmt
