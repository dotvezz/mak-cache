-include .env
export

clean:
	rm caddy

build:
	XCADDY_DEBUG=1 go tool xcaddy build --with $(shell awk '/^module/ {print $$2}' go.mod)=$(PWD)

debug: build
	go tool xcaddy --listen=:2345 --headless=true --api-version=2 --accept-multiclient exec ./caddy run

test:
	go test -v ./...

containers-up:
	cd ./testing/containers && docker compose up -d

containers-down:
	cd ./testing/containers && docker compose down

loadtest:
	go tool k6 run ./testing/k6/stress.ts
