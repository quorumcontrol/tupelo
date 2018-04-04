FROM golang:1.10.1 AS build

ENV RUSTUP_HOME=/usr/local/rustup \
    CARGO_HOME=/usr/local/cargo \
    PATH=/usr/local/cargo/bin:$PATH

RUN apt-get update && apt-get upgrade -y && apt-get -y install libssl-dev

RUN set -eux; \
    dpkgArch="$(dpkg --print-architecture)"; \
	case "${dpkgArch##*-}" in \
		amd64) rustArch='x86_64-unknown-linux-gnu'; rustupSha256='c9837990bce0faab4f6f52604311a19bb8d2cde989bea6a7b605c8e526db6f02' ;; \
		armhf) rustArch='armv7-unknown-linux-gnueabihf'; rustupSha256='297661e121048db3906f8c964999f765b4f6848632c0c2cfb6a1e93d99440732' ;; \
		arm64) rustArch='aarch64-unknown-linux-gnu'; rustupSha256='a68ac2d400409f485cb22756f0b3217b95449884e1ea6fd9b70522b3c0a929b2' ;; \
		i386) rustArch='i686-unknown-linux-gnu'; rustupSha256='27e6109c7b537b92a6c2d45ac941d959606ca26ec501d86085d651892a55d849' ;; \
		*) echo >&2 "unsupported architecture: ${dpkgArch}"; exit 1 ;; \
	esac; \
    \
    url="https://static.rust-lang.org/rustup/archive/1.11.0/${rustArch}/rustup-init"; \
    wget "$url"; \
    echo "${rustupSha256} *rustup-init" | sha256sum -c -; \
    chmod +x rustup-init; \
    ./rustup-init -y --no-modify-path --default-toolchain 1.25.0; \
    rm rustup-init; \
    chmod -R a+w $RUSTUP_HOME $CARGO_HOME; \
    rustup --version; \
    cargo --version; \
    rustc --version;

WORKDIR /go/src/github.com/quorumcontrol/qc3

COPY . .

RUN cd indy-crypto/libindy-crypto && cargo build --release && cd -

RUN cp /go/src/github.com/quorumcontrol/qc3/indy-crypto/libindy-crypto/target/release/*.so /usr/lib/

RUN go install -v


FROM debian:stretch-slim
RUN mkdir -p /var/lib/qc3

WORKDIR /var/lib/qc3

RUN apt-get update && \
    apt-get upgrade -y && \
    apt-get -y install libssl-dev && \
    rm -rf /var/lib/apt/lists/*

COPY --from=build /go/bin/qc3 /usr/bin/qc3
COPY --from=build /usr/lib/libindy_crypto.so /usr/lib/libindy_crypto.so

CMD ["bash"]
