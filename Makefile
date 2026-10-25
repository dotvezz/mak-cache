-include .env
export

clean\:caddy:
	rm caddy

build\:caddy:
	go tool xcaddy build \
		--with github.com/dotvezz/mak-cache/plugins/caddy=$(PWD)/plugins/caddy \
		--with github.com/dotvezz/mak-cache=$(PWD)

run\:caddy: build\:caddy
	./caddy run -c ./plugins/caddy/Caddyfile

build\:caddy-debug:
	XCADDY_DEBUG=1 make build

run\:caddy-debug: build\:caddy-debug
	go tool dlv --listen=:2345 --headless=true --api-version=2 --accept-multiclient exec ./caddy run

test:
	go test -v -cover ./...
	(cd plugins/caddy && go test -v -cover ./...)
