# syntax=docker/dockerfile:1

FROM golang:1.22-bookworm AS build

RUN apt-get update && apt-get install -y --no-install-recommends \
    libtdjson-dev && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY go go

RUN cd go && go build -o tg2sip-go

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    libtdjson && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=build /app/go/tg2sip-go /app/go/tg2sip-go
COPY settings.ini /app/settings.ini

WORKDIR /app/go

EXPOSE 5060/udp

CMD ["./tg2sip-go"]

