.PHONY: build test fmt lint dist install clean

VERSION ?= dev
LDFLAGS := -X main.version=$(VERSION)

build:
	go build -ldflags "$(LDFLAGS)" -o awt .

test:
	go test ./...

fmt:
	gofmt -w .

lint:
	gofmt -l .
	go vet ./...

dist:
	@mkdir -p dist
	GOOS=darwin  GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/awt-darwin-arm64 .
	GOOS=darwin  GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/awt-darwin-amd64 .
	GOOS=linux   GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/awt-linux-arm64 .
	GOOS=linux   GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/awt-linux-amd64 .
	GOOS=windows GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/awt-windows-arm64.exe .
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/awt-windows-amd64.exe .

install: build
	cp awt $(GOPATH)/bin/ 2>/dev/null || cp awt /usr/local/bin/

clean:
	rm -f awt
	rm -rf dist/ coverage.out
