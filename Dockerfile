# ============================================== BUILD EXECUTABLE MODULE ===============================================
FROM golang:alpine AS build
LABEL maintainer="Bojan Cekrlic <b.cekrlic@cargox.io>"

RUN apk add --no-cache upx git build-base
RUN mkdir -p /mapupdater/
WORKDIR /mapupdater/

COPY go.* /mapupdater/
RUN go mod download
COPY *.go /mapupdater/
RUN go build -ldflags="-linkmode external -extldflags -static" -o mapupdater
RUN upx -9 /mapupdater/mapupdater

# ================================================ BUILD MAIN MODULE ===================================================
FROM alpine:latest
LABEL maintainer="Bojan Cekrlic <b.cekrlic@cargox.io>"

COPY --from=build /mapupdater/mapupdater /mapupdater

ENTRYPOINT [ "/mapupdater" ]