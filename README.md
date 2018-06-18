## Dependencies

* cargo installed (rust CLI tool)
* [dep](https://github.com/golang/dep) installed (go dependency tool).

## Contributing

In order to run the code/tests you need to build the libindy-crypto rust library:

```
git submodule init && git submodule update
cd indy-crypto/libindy-crypto
cargo build --release
```

And grab the go dependencies:

```
dep ensure
```

If you change a protobuf code:

cd into the directory and
```
go generate
```

### Testing

Integration tests are not run by default, add the "integration" tag to run these: 

`go test -tags=integration ./...`
