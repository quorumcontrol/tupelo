FROM golang:1.12.6-alpine3.9 AS build

WORKDIR /app

RUN apk add --no-cache --update build-base

COPY . .

RUN go install -mod=vendor -v -a -ldflags '-extldflags "-static"' -gcflags=-trimpath="${PWD}" -asmflags=-trimpath="${PWD}"

FROM alpine:3.9
LABEL maintainer="dev@quorumcontrol.com"

COPY --from=build /go/bin/tupelo /usr/bin/tupelo

ENTRYPOINT ["/usr/bin/tupelo"]
