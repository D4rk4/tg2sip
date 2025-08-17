# syntax=docker/dockerfile:1

FROM ghcr.io/zelenin/tdlib-docker:b498497-alpine AS tdlib

FROM golang:1.24.1-alpine3.21 AS build
ENV LANG=en_US.UTF-8
ENV TZ=UTC
RUN set -eux && \
    apk update && \
    apk upgrade && \
    apk add \
        bash \
        build-base \
        ca-certificates \
        curl \
        git \
        linux-headers \
        openssl-dev \
        zlib-dev

WORKDIR /app
COPY --from=tdlib /usr/local/include/td /usr/local/include/td/
COPY --from=tdlib /usr/local/lib/libtd* /usr/local/lib/
COPY go go
RUN cd go && go build -o tg2sip-go

FROM alpine:3.21.3
ENV LANG=en_US.UTF-8
ENV TZ=UTC
RUN apk upgrade --no-cache && \
    apk add --no-cache \
        ca-certificates \
        libstdc++
WORKDIR /app
COPY --from=build /app/go/tg2sip-go /app/go/tg2sip-go
COPY settings.ini /app/settings.ini
WORKDIR /app/go
EXPOSE 5060/udp
CMD ["./tg2sip-go"]

