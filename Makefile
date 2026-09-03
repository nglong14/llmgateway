.PHONY: build run web test clean docker-up docker-down k6-smoke k6-baseline k6-baseline-free k6-stress k6-rate-limit hash-key migrate-up migrate-down migrate-status

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
	@test -n "$$GATEWAY_API_KEY" || (echo "Set GATEWAY_API_KEY (plaintext API key, not the hash) in your environment" && exit 1)
	k6 run loadtests/smoke.js

k6-baseline:
	@test -n "$$GATEWAY_API_KEY" || (echo "Set GATEWAY_API_KEY (plaintext API key, not the hash) in your environment" && exit 1)
	k6 run loadtests/baseline.js

k6-baseline-free:
	@test -n "$$GATEWAY_API_KEY" || (echo "Set GATEWAY_API_KEY (plaintext API key, not the hash) in your environment" && exit 1)
	k6 run loadtests/baseline-free.js

k6-stress:
	@test -n "$$GATEWAY_API_KEY" || (echo "Set GATEWAY_API_KEY (plaintext API key, not the hash) in your environment" && exit 1)
	k6 run loadtests/stress.js

k6-rate-limit:
	k6 run loadtests/rate_limit.js

hash-key:
	@if [ -n "$(KEY)" ]; then \
		printf '%s' "$(KEY)" | sha256sum | cut -d' ' -f1; \
	else \
		bash -c 'read -s -p "Enter API key: " key; echo; printf "%s" "$$key" | sha256sum | cut -d" " -f1'; \
	fi

migrate-up:
	go run ./cmd/migrate --config configs/gateway.yaml up

migrate-down:
	go run ./cmd/migrate --config configs/gateway.yaml down

migrate-status:
	go run ./cmd/migrate --config configs/gateway.yaml status
