-include .env
export

clean:
	rm caddy

build:
	XCADDY_DEBUG=1 go tool xcaddy build --with $(shell awk '/^module/ {print $$2}' go.mod)=$(PWD)

debug: build
	go tool dlv --listen=:2345 --headless=true --api-version=2 --accept-multiclient exec ./caddy run

test:
	go test -v -cover ./...
	(cd plugins/caddy && go test -v -cover ./...)
