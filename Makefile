BINARY=act

build:
	go build -ldflags "-X main.version=dev" -o $(BINARY) .

install: build
	mv $(BINARY) /usr/local/bin/$(BINARY)

clean:
	rm -f $(BINARY)

.PHONY: build install clean
