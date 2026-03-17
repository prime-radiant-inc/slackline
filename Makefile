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
	git diff --exit-code HEAD && git diff --cached --exit-code || (echo "Uncommitted changes — commit first" && exit 1)
	git tag v$(VERSION) && git push origin v$(VERSION)
