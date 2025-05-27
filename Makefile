watch:
	go tool pulse

build:
	go build -o bin/pong .

clean:
	rm -rf bin

.PHONY: watch build clean