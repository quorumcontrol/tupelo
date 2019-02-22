FROM alpine:3.9
LABEL maintainer="dev@quroumcontrol.com"

RUN mkdir -p /var/lib/tupelo

WORKDIR /var/lib/tupelo

ARG VERSION=snapshot

COPY bin/tupelo-${VERSION}-linux-amd64 /usr/bin/tupelo

ENTRYPOINT ["/usr/bin/tupelo"]
