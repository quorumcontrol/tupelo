VERSION ?= snapshot
ifeq ($(VERSION), snapshot)
	TAG = latest
else
	TAG = $(VERSION)
endif

MD5CMD = $(shell { command -v md5sum || command -v md5; } 2>/dev/null)

# GUARD is a function which calculates md5 sum for its
# argument variable name.
GUARD = $(1)_GUARD_$(shell echo $($(1)) | $(MD5CMD) | cut -d ' ' -f 1)

FIRSTGOPATH = $(firstword $(subst :, ,$(GOPATH)))
VERSION_TXT = resources/templates/version.txt

generated = wallet/walletrpc/service.pb.go
gosources = $(shell find . -path "./vendor/*" -prune -o -type f -name "*.go" -print)
packr = packrd/packed-packr.go resources/resources-packr.go

all: tupelo

$(VERSION_TXT): $(call GUARD,VERSION)
	mkdir -p resources/templates
	echo $(VERSION) > $@

$(call GUARD,VERSION):
	rm -rf VERSION_GUARD_*
	touch $@

$(FIRSTGOPATH)/bin/protoc-gen-go:
	go get -u github.com/golang/protobuf/protoc-gen-go

$(FIRSTGOPATH)/bin/msgp:
	go get -u github.com/tinylib/msgp

$(FIRSTGOPATH)/bin/packr2:
	go get -u github.com/gobuffalo/packr/v2/packr2

$(packr): $(FIRSTGOPATH)/bin/packr2 $(VERSION_TXT)
	$(FIRSTGOPATH)/bin/packr2

$(generated): wallet/walletrpc/service.proto $(FIRSTGOPATH)/bin/msgp $(FIRSTGOPATH)/bin/protoc-gen-go
	cd wallet/walletrpc && go generate

vendor: go.mod go.sum $(FIRSTGOPATH)/bin/modvendor
	go mod vendor
	modvendor -copy="**/*.c **/*.h"

tupelo: $(packr) $(generated) $(gosources) go.mod go.sum
	go build

lint: $(FIRSTGOPATH)/bin/golangci-lint
	$(FIRSTGOPATH)/bin/golangci-lint run --build-tags integration

$(FIRSTGOPATH)/bin/golangci-lint:
	./scripts/download-golangci-lint.sh

test: $(packr) $(generated) $(gosources) go.mod go.sum
	go test ./... -tags=integration

integration-test: test .tupelo-integration.yml
	docker run -v /var/run/docker.sock:/var/run/docker.sock -v ${CURDIR}:/src quorumcontrol/tupelo-integration-runner

ci-test: $(packr) $(generated) $(gosources) go.mod go.sum
	go test -mod=readonly ./... -tags=integration

ci-integration-test: ci-test integration-test

docker-image: vendor $(packr) $(generated) $(gosources) Dockerfile .dockerignore
	docker build -t quorumcontrol/tupelo:$(TAG) .

$(FIRSTGOPATH)/bin/modvendor:
	go get -u github.com/goware/modvendor

install: $(packr) $(generated) $(gosources) go.mod go.sum
	go install -a -gcflags=-trimpath=$(GOPATH) -asmflags=-trimpath=$(GOPATH)

clean:
	$(FIRSTGOPATH)/bin/packr2 clean
	go clean
	rm -rf vendor

github-prepare:
	# mimic https://github.com/actions/docker/blob/b12ae68bebbb2781edb562c0260881a3f86963b4/tag/tag.rb#L39
	VERSION=$(shell { echo $(GITHUB_REF) | rev | cut -d / -f 1 | rev; }) $(MAKE) $(packr)

.PHONY: all test integration-test ci-test ci-integration-test docker-image clean install lint github-prepare
