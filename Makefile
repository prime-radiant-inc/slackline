.PHONY: build test vet clean

build:
	go build -o slackline .

test:
	go test ./... -v

vet:
	go vet ./...

clean:
	rm -f slackline
