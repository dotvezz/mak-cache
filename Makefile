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
	go tool k6 run --log-format raw ./testing/k6/e2e.ts

e2e-all:
	go tool k6 run --log-format raw -e ALL=true ./testing/k6/e2e.ts

benchmark:
	go tool k6 run ./testing/k6/benchmark.ts

benchmark-souin:
	PORT=8082 go tool k6 run ./testing/k6/benchmark.ts

benchmark-varnish:
	PORT=8083 go tool k6 run ./testing/k6/benchmark.ts

#benchmark-all:
#	@for target in "caddy-otter:8086" "caddy-valkey:8087" "caddy-souin:8082" "varnish:8083"; do \
#		name=$${target%%:*}; \
#		port=$${target##*:}; \
#		echo "=================================================="; \
#		echo " Benchmarking $$name (port $$port)"; \
#		echo "=================================================="; \
#		PORT=$$port go tool k6 run ./testing/k6/benchmark.ts || exit 1; \
#	done
