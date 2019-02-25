package walletrpc

//go:generate protoc -I=. -I=$GOPATH/src --go_out=plugins=grpc:. service.proto

// RUN `go generate` in this directory when updating the service.proto. Requires the protoc command to be in your path.
