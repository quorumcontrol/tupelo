package consensuspb

//go:generate protoc -I=$GOPATH/src -I=$GOPATH/src/github.com/gogo/protobuf/protobuf -I=. --gogoslick_out=Mgoogle/protobuf/timestamp.proto=github.com/gogo/protobuf/types,Mgoogle/protobuf/any.proto=github.com/gogo/protobuf/types:. consensus.proto
