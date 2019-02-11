package walletrpc

//go:generate protoc -I. -I$GOPATH/src -I$GOPATH/src/github.com/grpc-ecosystem/grpc-gateway/third_party/googleapis --go_out=plugins=grpc:. --grpc-gateway_out=logtostderr=true:. service.proto

//RUN `go generate` in this directory when updating the service.proto. Requires the protoc command to be in your path.
