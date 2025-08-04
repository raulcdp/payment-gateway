build-image:
	docker build -t payment-gateway:latest .

build-server:
	go build -a -tags=jsoniter -tags=nomsgpack ./cmd/server

build-worker:
	go build -a ./cmd/server

run-server:
	go run -tags=jsoniter -tags=nomsgpack ./cmd/server

up:
	docker compose down
	docker compose up -d --build

test: up
	k6 run rinha-test/rinha.js

dry-test:
	k6 run rinha-test/rinha.js