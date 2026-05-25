BINARY=act

build:
	go build -o $(BINARY) .

install: build
	mv $(BINARY) /usr/local/bin/$(BINARY)

clean:
	rm -f $(BINARY)

.PHONY: build install clean
