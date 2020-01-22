SHELL = /bin/bash -o pipefail

BUMP_VERSION := $(GOPATH)/bin/bump_version
MEGACHECK := $(GOPATH)/bin/megacheck

vet: | $(MEGACHECK)
	go vet ./...
	$(MEGACHECK) --ignore='github.com/kevinburke/handlers/*.go:S1002' ./...

$(MEGACHECK):
	go get honnef.co/go/tools/cmd/megacheck

test: vet
	go test -timeout=10s ./...

race-test: vet
	go test -race -timeout=10s ./...

install:
	go install ./...

$(BUMP_VERSION):
	go get github.com/Shyp/bump_version

release: race-test | $(BUMP_VERSION)
	$(BUMP_VERSION) minor lib.go
