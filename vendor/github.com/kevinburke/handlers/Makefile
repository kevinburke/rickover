SHELL = /bin/bash -o pipefail

BUMP_VERSION := $(GOPATH)/bin/bump_version

vet:
	go vet ./...
	staticcheck ./...

test: vet
	go test -timeout=10s ./...

install-ci:
	go get honnef.co/go/tools/cmd/staticcheck
	go get -t ./...

ci: install-ci vet race-test

race-test: vet
	go test -race -timeout=10s ./...

install:
	go install ./...

$(BUMP_VERSION):
	go get github.com/kevinburke/bump_version

release: race-test | $(BUMP_VERSION)
	$(BUMP_VERSION) minor lib.go
