# ================ BUILD EXECUTABLE MODULE ================
FROM golang:alpine AS build
LABEL maintainer="Bojan Cekrlic <b.cekrlic@cargox.io>"

RUN apk add --no-cache upx git build-base
RUN mkdir -p /mapupdater/
WORKDIR /mapupdater/

COPY go.* *.go /mapupdater/
RUN go get ./...
RUN go build -ldflags="-linkmode external -extldflags" -o mapupdater main.go
RUN upx -9 /mapupdater/mapupdater

# ================ BUILD MAIN MODULE ================
FROM scratch
LABEL maintainer="Bojan Cekrlic <b.cekrlic@cargox.io>"

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /mapupdater/mapupdater /mapupdater

ENTRYPOINT [ "/mapupdater" ]