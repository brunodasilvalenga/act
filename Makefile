BINARY=act

build:
	go build -ldflags "-X main.version=dev" -o $(BINARY) .

install: build
	mv $(BINARY) /usr/local/bin/$(BINARY)

clean:
	rm -f $(BINARY)

fmt:
	@test -z "$$(gofmt -l .)" || (echo "The following files are not gofmt'd:"; gofmt -l .; exit 1)

vet:
	go vet ./...

test: fmt vet
	go test ./...

.PHONY: build install clean fmt vet test
