FROM golang:alpine as builder

RUN apk update && apk add --no-cache git

WORKDIR /src

ENV GO111MODULE=on

# Fetch dependencies first
COPY ./go.mod ./go.sum ./
RUN go mod download

COPY ./ ./

WORKDIR /src/cmd/client
RUN CGO_ENABLED=0 go install -installsuffix 'static' 

RUN adduser -D -g '' pion

FROM scratch

COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /go/bin/client /client

USER pion

ENTRYPOINT ["/client"]

# docker build -t turn-client -f Dockerfile.client .
# docker run turn-client