SHELL = /bin/bash -o pipefail

BUMP_VERSION := $(GOPATH)/bin/bump_version
STATICCHECK := $(GOPATH)/bin/staticcheck

vet: | $(STATICCHECK)
	go vet ./...
	$(STATICCHECK) ./...

$(STATICCHECK):
	go get honnef.co/go/tools/cmd/staticcheck

test: vet
	go test -timeout=10s ./...

race-test: vet
	go test -race -timeout=10s ./...

install:
	go install ./...

$(BUMP_VERSION):
	go get github.com/kevinburke/bump_version

release: race-test | $(BUMP_VERSION)
	$(BUMP_VERSION) minor lib.go
