VERSION ?= snapshot

FIRSTGOPATH = $(firstword $(subst :, ,${GOPATH}))

gosources = $(shell find . -path "./vendor/*" -prune -o -type f -name "*.go" -print)

protobuf = wallet/walletrpc/service.pb.go
msgpack = gossip3/messages/external_gen.go gossip3/remote/messages_gen.go
generated = $(protobuf) $(msgpack)

binaries = bin/tupelo-${VERSION}-linux-amd64 bin/tupelo-${VERSION}-darwin-amd64 \
           bin/tupelo-${VERSION}-windows-amd64.exe

all: build

$(binaries): vendor $(generated) ${FIRSTGOPATH}/bin/xgo $(gosources)
	${FIRSTGOPATH}/bin/xgo --targets=darwin-10.10/amd64,linux/amd64,windows-6.0/amd64 \
	--out tupelo-${VERSION} ./
	mv tupelo-${VERSION}-darwin-*-amd64 bin/tupelo-${VERSION}-darwin-amd64
	mv tupelo-${VERSION}-linux-amd64 bin/
	mv tupelo-${VERSION}-windows-*-amd64.exe bin/tupelo-${VERSION}-windows-amd64.exe

build: $(binaries)

protobuf: $(protobuf)

msgpack: $(msgpack)

test: vendor $(generated)
	go test ./... -tags=integration -timeout=2m

run: vendor $(generated)
	go run main.go rpc-server

docker-image: build .dockerignore
	docker build --build-arg VERSION=${VERSION} -t quorumcontrol/tupelo:${VERSION} .

release: all release/tupelo-${VERSION}.zip docker-image
	git tag -s ${VERSION}
	git push origin ${VERSION}
	docker push quorumcontrol/tupelo:${VERSION}
	# TODO: Upload zip file & checksums to GitHub release page

release/tupelo-${VERSION}.zip: all
	zip release/tupelo-${VERSION}.zip -r bin/

zip: release/tupelo-${VERSION}.zip

# Generic protobuf rule
%.pb.go: %.go %.proto ${FIRSTGOPATH}/bin/protoc-gen-go
	go generate $<

# Generic msgpack rule
%_gen.go: %.go
	go generate $<

vendor: Gopkg.toml Gopkg.lock
	dep ensure

deps: vendor ${FIRSTGOPATH}/bin/protoc-gen-go ${FIRSTGOPATH}/bin/xgo

${FIRSTGOPATH}/bin/xgo:
	go get -u github.com/karalabe/xgo

${FIRSTGOPATH}/bin/protoc-gen-go:
	@rm -rf ${FIRSTGOPATH}/src/github.com/golang/protobuf
	go get -d -u github.com/golang/protobuf/protoc-gen-go
	git -C ${FIRSTGOPATH}/src/github.com/golang/protobuf checkout v1.2.0
	go install github.com/golang/protobuf/protoc-gen-go

clean:
	go clean
	rm -rf vendor

.PHONY: all deps build test run zip release docker-image clean