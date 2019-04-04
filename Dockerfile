FROM golang:1.11.5 AS build

WORKDIR /app

COPY . .

RUN go install -mod=vendor -v -a -ldflags '-extldflags "-static"' -gcflags=-trimpath=$GOPATH -asmflags=-trimpath=$GOPATH

FROM alpine:3.9
LABEL maintainer="dev@quorumcontrol.com"

RUN mkdir -p /var/lib/tupelo

WORKDIR /var/lib/tupelo

COPY --from=build /go/bin/tupelo /usr/bin/tupelo

ENTRYPOINT ["/usr/bin/tupelo"]
