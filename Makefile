BINARY_NAME := gnucash-mcp

.PHONY: build clean run test

build:
	go build -o $(BINARY_NAME) .

clean:
	rm -f $(BINARY_NAME)

test:
	go test ./...

run: build
	./$(BINARY_NAME)