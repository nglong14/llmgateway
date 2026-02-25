.PHONY: build run test clean

build:
	go build -o bin/gateway.exe ./cmd/gateway

run:
	go run ./cmd/gateway --config configs/gateway.yaml

test:
	go test ./... -v -count=1 -race

clean:
	rm -rf bin/
