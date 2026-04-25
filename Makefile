DEV_SERVICES := postgres redis
AIR_BIN := $(shell command -v air 2>/dev/null || printf "%s/bin/air" "$$(go env GOPATH)")

.PHONY: dev-db dev-api dev-api-run install-air dev-stop dev-status test

dev-db:
	docker compose up -d $(DEV_SERVICES)
	-docker compose stop app

dev-api:
	@if [ ! -x "$(AIR_BIN)" ]; then \
		echo "ยังไม่พบ air ให้รัน: make install-air"; \
		exit 1; \
	fi
	$(AIR_BIN)

dev-api-run:
	go run .

install-air:
	go install github.com/air-verse/air@latest

dev-stop:
	docker compose stop $(DEV_SERVICES)

dev-status:
	docker compose ps

test:
	go test ./...
