# SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
# SPDX-License-Identifier: MIT

FROM golang:alpine as builder

ARG VERSION=master

RUN apk add --no-cache git

WORKDIR /build
# Clone Source using GIT
RUN git clone --branch=$VERSION --depth=1 https://github.com/pion/turn.git turn && rm -rf turn/.git

WORKDIR /build/turn/examples/turn-server/simple

# Download all the dependencies
RUN go get -d -v ./...

# Build static binary
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-w -s" -o turn-server main.go


##### main
FROM alpine

ARG BUILD_DATE
ARG VCS_REF
ARG VERSION=master

LABEL org.label-schema.build-date="${BUILD_DATE}" \
      org.label-schema.name="pion-turn" \
      org.label-schema.description="A toolkit for building TURN clients and servers in Go" \
      org.label-schema.usage="https://github.com/pion/turn#readme" \
      org.label-schema.vcs-ref="${VCS_REF}" \
      org.label-schema.vcs-url="https://github.com/pion/turn" \
      org.label-schema.vendor="Sean-Der" \
      org.label-schema.version="${VERSION}" \
      maintainer="https://github.com/pion"

ENV REALM localhost
ENV USERS username=password
ENV UDP_PORT 3478
ENV PUBLIC_IP 127.0.0.1

EXPOSE 3478
#EXPOSE 49152:65535/tcp
#EXPOSE 49152:65535/udp

USER nobody

# Copy the executable
COPY --from=builder /build/turn/examples/turn-server/simple/turn-server /usr/bin/

# Run the executable
CMD turn-server -public-ip $PUBLIC_IP -users $USERS -realm $REALM -port $UDP_PORT

# docker build -t pion-turn -f Dockerfile .
# docker run --rm -e REALM="localhost" -e USERS="username=password" -e UDP_PORT="3478" -e PUBLIC_IP="127.0.0.1" -p 3478:3478 pion-turn
