## Dependencies

* [dep](https://github.com/golang/dep) installed (go dependency tool).

## Contributing

Grab the go dependencies:

```
dep ensure
```

If you change a protobuf code:

cd into the directory and
```
go generate
```

### Testing

Integration tests are not run by default, add the "integration" tag to run
these. Also add the path containing the libindy-crypto library as an ldflag so
the linker can find it:

`go test -tags=integration -ldflags="-r /path/to/libindy-crypto/release" ./...`
