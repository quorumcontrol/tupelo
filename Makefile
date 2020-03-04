VERSION ?= snapshot
ifeq ($(VERSION), snapshot)
	TAG = latest
else
	TAG = $(VERSION)
endif

# This is important to export until we're on Go 1.13+ or packr can break
export GO111MODULE = on

MD5CMD = $(shell { command -v md5sum || command -v md5; } 2>/dev/null)

# GUARD is a function which calculates md5 sum for its
# argument variable name.
GUARD = $(1)_GUARD_$(shell echo $($(1)) | $(MD5CMD) | cut -d ' ' -f 1)

FIRSTGOPATH = $(firstword $(subst :, ,$(GOPATH)))
VERSION_TXT = resources/templates/version.txt

gosources = $(shell find . -path "./vendor/*" -prune -o -type f -name "*.go" -print)
packr = packrd/packed-packr.go resources/resources-packr.go

all: tupelo

$(VERSION_TXT): $(call GUARD,VERSION)
	mkdir -p resources/templates
	echo $(VERSION) > $@

$(call GUARD,VERSION):
	rm -rf VERSION_GUARD_*
	touch $@

$(FIRSTGOPATH)/bin/packr2:
	GO111MODULE=off go get -u github.com/gobuffalo/packr/v2/packr2

$(packr): $(FIRSTGOPATH)/bin/packr2 $(VERSION_TXT)
	$(FIRSTGOPATH)/bin/packr2

vendor: go.mod go.sum $(FIRSTGOPATH)/bin/modvendor
	go mod vendor
	$(FIRSTGOPATH)/bin/modvendor -copy="**/*.c **/*.h"

tupelo: $(packr) $(gosources) go.mod go.sum
	go build

lint: $(FIRSTGOPATH)/bin/golangci-lint $(packr)
	$(FIRSTGOPATH)/bin/golangci-lint run --build-tags integration

$(FIRSTGOPATH)/bin/golangci-lint:
	./scripts/download-golangci-lint.sh

test: $(packr) $(gosources) go.mod go.sum
	go test -tags=integration ./...

docker-image: vendor $(packr) $(gosources) Dockerfile .dockerignore
	docker build -t quorumcontrol/tupelo:$(TAG) .

$(FIRSTGOPATH)/bin/modvendor:
	GO111MODULE=off go get -u github.com/goware/modvendor

install: $(packr) $(gosources) go.mod go.sum
	go install -a -gcflags=-trimpath=$(CURDIR) -asmflags=-trimpath=$(CURDIR)

clean: $(FIRSTGOPATH)/bin/packr2 
	$(FIRSTGOPATH)/bin/packr2 clean
	go clean
	rm -rf vendor

github-prepare:
	# mimic https://github.com/actions/docker/blob/b12ae68bebbb2781edb562c0260881a3f86963b4/tag/tag.rb#L39
	VERSION=$(shell { echo $(GITHUB_REF) | rev | cut -d / -f 1 | rev; }) $(MAKE) $(packr)

.PHONY: all test ci-test docker-image clean install lint github-prepare generate
