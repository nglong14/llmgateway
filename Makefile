.PHONY: build run test clean docker-up docker-down k6-smoke k6-stress k6-rate-limit

build:
	go build -o bin/gateway.exe ./cmd/gateway

run:
	go run ./cmd/gateway --config configs/gateway.yaml

test:
	go test ./... -v -count=1 -race

clean:
	rm -rf bin/

docker-up:
	docker compose up --build -d

docker-down:
	docker compose down

k6-smoke:
	k6 run loadtests/smoke.js

k6-stress:
	k6 run loadtests/stress.js

k6-rate-limit:
	k6 run loadtests/rate_limit.js
