.PHONY: build test vet clean release

build:
	go build -o slackline .

test:
	go test ./... -v

vet:
	go vet ./...

clean:
	rm -f slackline

release:
ifndef VERSION
	$(error VERSION is required. Usage: make release VERSION=0.1.0)
endif
	if ! git diff --exit-code HEAD || ! git diff --cached --exit-code; then echo "Uncommitted changes — commit first"; false; fi
	git tag v$(VERSION) && git push origin v$(VERSION)
