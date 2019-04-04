FROM alpine:latest

ENV GOPATH /usr/local
ENV REALM localhost
ENV USERS username=password
ENV UDP_PORT 3478

RUN apk --no-cache add go git musl-dev && rm -rf /var/cache/apk/*
RUN go get github.com/cespare/reflex github.com/pion/turn

WORKDIR /usr/local/src/github.com/pion/turn
ENTRYPOINT ["/usr/local/bin/reflex"]
CMD ["-r", ".", "-s", "go", "run", "cmd/simple-turn/main.go"]
