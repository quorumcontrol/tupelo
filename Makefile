VERSION ?= snapshot
ifeq ($(VERSION), snapshot)
	TAG = latest
else
	TAG = $(VERSION)
endif

FIRSTGOPATH = $(firstword $(subst :, ,$(GOPATH)))

generated = gossip3/messages/internal_gen.go gossip3/messages/internal_gen_test.go
gosources = $(shell find . -path "./vendor/*" -prune -o -type f -name "*.go" -print)

all: tupelo

$(generated): gossip3/messages/internal.go
	cd gossip3/messages && go generate

vendor: Gopkg.toml Gopkg.lock
	dep ensure

tupelo: vendor $(generated) $(gosources)
	go build

test: vendor $(generated) $(gosources)
	go test ./...

integration-test: vendor $(generated) $(gosources)
	go test ./... -tags=integration

docker-image: vendor Dockerfile .dockerignore
	docker build -t quorumcontrol/tupelo:$(TAG) .

install: vendor $(generated) $(gosources)
	go install -a -gcflags=-trimpath=$(GOPATH) -asmflags=-trimpath=$(GOPATH)

clean:
	go clean
	rm -rf vendor

.PHONY: all test integration-test docker-image clean install
