.PHONY: build build-go build-ts test test-go test-ts lint fmt clean

# Build both the Go engine and the TS SDK.
build: build-go build-ts

build-go:
	$(MAKE) -C go build

build-ts:
	cd ts && bun install && bun run build

test: test-go test-ts

test-go:
	$(MAKE) -C go test

# The committed ts tests are hermetic (they use a fake binary fixture), so this
# needs neither Go nor a network.
test-ts:
	cd ts && bun install && bun run typecheck && bun test

lint:
	$(MAKE) -C go lint
	cd ts && bun run typecheck

fmt:
	$(MAKE) -C go fmt

clean:
	$(MAKE) -C go clean
	rm -rf ts/node_modules ts/dist
