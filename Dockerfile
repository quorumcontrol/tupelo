FROM golang:1.13.6-alpine3.11 AS build

WORKDIR /app

RUN apk add --no-cache --update build-base

COPY . .

RUN go install -mod=vendor -v -a -gcflags=-trimpath="${PWD}" -asmflags=-trimpath="${PWD}"

FROM alpine:3.11
LABEL maintainer="dev@quorumcontrol.com"

RUN apk add --no-cache --update gettext ca-certificates curl libcap

RUN mkdir -p /tupelo

RUN addgroup -g 1000 tupelo && \
    adduser -u 1000 -G tupelo -h /tupelo --disabled-password tupelo && \
    chmod 0775 /tupelo && \
    chgrp 0 /tupelo

COPY --from=build /go/bin/tupelo /usr/bin/tupelo
RUN setcap 'cap_net_bind_service=+ep' /usr/bin/tupelo

COPY ./docker/docker-entrypoint.sh /usr/bin/docker-entrypoint
RUN chmod +x /usr/bin/docker-entrypoint

COPY --chown=1000:0 ./docker/config.toml.tpl /tupelo/config.toml.tpl

USER tupelo

ENTRYPOINT ["/usr/bin/docker-entrypoint"]
