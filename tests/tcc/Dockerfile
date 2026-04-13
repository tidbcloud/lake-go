FROM golang:latest AS builder

WORKDIR /usr/src

COPY . .

RUN CGO_ENABLED=0 go build -tags rows_hack ./tests/ttc

FROM debian:bullseye-slim

COPY --from=builder  /usr/src/ttc /usr/local/bin/ttc-server

# docker run --net host -e TTC_PORT=9093 -e LAKE_DSN=lake://default:@127.0.0.1:8000 ghcr.io/tidbcloud/ttc-go:latest
CMD ["ttc-server"]