-include .env
export

clean:
	rm caddy

build:
	XCADDY_DEBUG=1 go tool xcaddy build --with $(shell awk '/^module/ {print $$2}' go.mod)=$(PWD)

debug: build
	go tool dlv --listen=:2345 --headless=true --api-version=2 --accept-multiclient exec ./caddy run

test:
	go test -v ./...

up:
	cd ./testing/containers && docker compose up -d

down:
	cd ./testing/containers && docker compose down

build-containers:
	cd ./testing/containers && docker compose build

loadtest:
	go tool k6 run ./testing/k6/stress.ts

e2e:
	go tool k6 run ./testing/k6/e2e.ts

