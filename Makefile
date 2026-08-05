.PHONY: build run web test clean docker-up docker-down k6-smoke k6-stress k6-rate-limit hash-key migrate-up migrate-down migrate-status

build:
	go build -o bin/gateway.exe ./cmd/gateway

run:
	go run ./cmd/gateway --config configs/gateway.yaml

web:
	npm --prefix web run dev

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

hash-key:
	@read -s -p "Enter API key: " key; echo; echo -n "$$key" | sha256sum | cut -d' ' -f1

migrate-up:
	go run ./cmd/migrate --config configs/gateway.yaml up

migrate-down:
	go run ./cmd/migrate --config configs/gateway.yaml down

migrate-status:
	go run ./cmd/migrate --config configs/gateway.yaml status
