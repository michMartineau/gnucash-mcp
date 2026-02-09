BINARY_NAME := gnucash-mcp

.PHONY: build clean run

build:
	go build -o $(BINARY_NAME) .

clean:
	rm -f $(BINARY_NAME)

run: build
	./$(BINARY_NAME)