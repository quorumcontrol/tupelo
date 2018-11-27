FROM golang:1.10.1 AS build

WORKDIR /go/src/github.com/quorumcontrol/tupelo

COPY . .

RUN go install -v -a -ldflags '-extldflags "-static"' -gcflags=-trimpath=$GOPATH -asmflags=-trimpath=$GOPATH

FROM debian:stretch-slim
RUN mkdir -p /var/lib/tupelo

WORKDIR /var/lib/tupelo

COPY --from=build /go/bin/tupelo /usr/bin/tupelo

CMD ["bash"]
