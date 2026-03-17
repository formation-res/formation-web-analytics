SHELL := /bin/bash

.PHONY: test test-backend test-elasticsearch smoke-test smoke-test-browser-client stack-up stack-down bootstrap fmt check

test: test-backend

test-backend:
	go test -count=1 -timeout 10s ./...

test-elasticsearch:
	docker compose -f docker-compose.elasticsearch.yml up -d
	until curl -fsS 'http://localhost:19920/_cluster/health' >/dev/null; do sleep 2; done
	./scripts/create-data-stream-and-templates.sh

smoke-test:
	./scripts/smoke-test-stack.sh

smoke-test-browser-client:
	bash ./scripts/smoke-test-browser-client.sh

stack-up:
	docker compose up --build -d

stack-down:
	docker compose down

bootstrap:
	./scripts/create-data-stream-and-templates.sh

fmt:
	gofmt -w $$(find cmd internal -name '*.go' -type f)

check: fmt test-backend
