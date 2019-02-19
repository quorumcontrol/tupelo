FROM golang:1.11.5

LABEL "com.github.actions.name"="Golang Dep"
LABEL "com.github.actions.description"="Run dep ensure with access to private repos"
LABEL "com.github.actions.icon"="download"
LABEL "com.github.actions.color"="purple"

SHELL ["/bin/bash", "-c"]

RUN mkdir -p ~/.ssh && \
    ssh-keyscan -t rsa github.com > github.pub && \
    diff <(ssh-keygen -lf github.pub) <(echo "2048 SHA256:nThbg6kXUpJWGl7E1IGOCspRomTxdCARLviKw6E5SY8 github.com (RSA)") && \
    cat github.pub >> ~/.ssh/known_hosts && \
    rm -f github.pub

RUN curl https://raw.githubusercontent.com/golang/dep/master/install.sh | sh

ADD entrypoint.sh /entrypoint.sh

RUN chmod +x /entrypoint.sh

ENV DEPPROJECTROOT=/github/workspace

WORKDIR /github/workspace

ENTRYPOINT ["/entrypoint.sh"]

CMD ["ensure"]
