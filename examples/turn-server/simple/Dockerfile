# SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
# SPDX-License-Identifier: MIT

FROM golang:latest
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

ENV GOPATH /usr/local
ENV REALM localhost
ENV USERS username=password
ENV UDP_PORT 3478
ENV PUBLIC_IP 127.0.0.1

# Use turn-server/simple by default
ENV EXAMPLE_DIR examples/turn-server
ENV EXAMPLE_BIN simple

WORKDIR /usr/local/src/github.com/pions

# Clone Source using GIT
RUN git clone --branch=$VERSION --depth=1 https://github.com/pion/turn.git turn && rm -rf turn/.git

WORKDIR /usr/local/src/github.com/pions/turn/$EXAMPLE_DIR

# Download all the dependencies
RUN go get -d -v ./...

# Install the package
RUN go install -v ./...

EXPOSE 3478
#EXPOSE 49152:65535/tcp
#EXPOSE 49152:65535/udp

RUN chown -R nobody:nogroup ./ && chmod -R a-w ./
USER nobody

# Run the executable
CMD $EXAMPLE_BIN -public-ip $PUBLIC_IP -users $USERS -realm $REALM -port $UDP_PORT

# docker build -t pion-turn -f Dockerfile .
# docker run -e REALM="localhost" -e USERS="username=password" -e UDP_PORT="3478" -e PUBLIC_IP="127.0.0.1" -p 3478:3478 pion-turn
