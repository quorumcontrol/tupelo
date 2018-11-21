FROM golang:1.10.1 AS build

WORKDIR /go/src/github.com/quorumcontrol/qc3

COPY . .

RUN go install -v -a -ldflags '-extldflags "-static"' -gcflags=-trimpath=$GOPATH -asmflags=-trimpath=$GOPATH

FROM debian:stretch-slim
RUN mkdir -p /var/lib/qc3

WORKDIR /var/lib/qc3

COPY --from=build /go/bin/qc3 /usr/bin/qc3

CMD ["bash"]
