VERSION ?= snapshot
ifeq ($(VERSION), snapshot)
	TAG = latest
else
	TAG = $(VERSION)
endif

# GUARD is a function which calculates md5 sum for its
# argument variable name.
GUARD = $(1)_GUARD_$(shell echo $($(1)) | md5sum | cut -d ' ' -f 1)

FIRSTGOPATH = $(firstword $(subst :, ,$(GOPATH)))
VERSION_TXT = resources/templates/version.txt

generated = gossip3/messages/internal_gen.go gossip3/messages/internal_gen_test.go
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
	go get -u github.com/gobuffalo/packr/v2/packr2

$(packr): $(FIRSTGOPATH)/bin/packr2 $(VERSION_TXT)
	$(FIRSTGOPATH)/bin/packr2

$(generated): gossip3/messages/internal.go
	cd gossip3/messages && go generate

vendor: Gopkg.toml Gopkg.lock
	dep ensure

tupelo: vendor $(packr) $(generated) $(gosources)
	go build

test: vendor $(packr) $(generated) $(gosources)
	go test ./...

integration-test: vendor $(packr) $(generated) $(gosources)
	go test ./... -tags=integration

docker-image: vendor $(packr) $(generated) $(gosources) Dockerfile .dockerignore
	docker build -t quorumcontrol/tupelo:$(TAG) .

install: vendor $(packr) $(generated) $(gosources)
	go install -a -gcflags=-trimpath=$(GOPATH) -asmflags=-trimpath=$(GOPATH)

clean:
	$(FIRSTGOPATH)/bin/packr2 clean
	go clean
	rm -rf vendor

.PHONY: all test integration-test docker-image clean install
