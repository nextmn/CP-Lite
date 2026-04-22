# Copyright Louis Royer and the NextMN contributors. All rights reserved.
# Use of this source code is governed by a MIT-style license that can be
# found in the LICENSE file.
# SPDX-License-Identifier: MIT

FROM golang:1.26.1 AS builder
WORKDIR /src
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 go build -o /usr/local/bin/cp-lite

FROM alpine:3.23.3
RUN apk add --no-cache iptables iproute2
COPY --from=builder /usr/local/bin/cp-lite /usr/local/bin/cp-lite
ENTRYPOINT ["cp-lite"]
CMD ["--help"]
HEALTHCHECK --interval=1m --timeout=1s --retries=3 --start-period=5s --start-interval=100ms \
CMD ["cp-lite", "healthcheck"]
