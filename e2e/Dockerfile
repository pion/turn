# SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
# SPDX-License-Identifier: MIT

FROM docker.io/library/golang:1.27-trixie

RUN apt-get update && apt-get install -y coturn

COPY . /go/src/github.com/pion/turn
WORKDIR /go/src/github.com/pion/turn/e2e

CMD ["go", "test", "-tags=coturn", "-v", "."]
