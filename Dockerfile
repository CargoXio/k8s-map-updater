# ============================================== BUILD EXECUTABLE MODULE ===============================================
FROM golang:alpine AS build
LABEL maintainer="Bojan Cekrlic <b.cekrlic@cargox.io>"

RUN apk add --no-cache git build-base
RUN mkdir -p /mapupdater/
WORKDIR /mapupdater/

COPY go.* /mapupdater/
RUN go mod download
COPY *.go /mapupdater/
RUN go build -ldflags="-linkmode external -extldflags -static" -o mapupdater
RUN upx -9 /mapupdater/mapupdater

# ================================================ BUILD MAIN MODULE ===================================================
# ================ linux/386 ================
FROM --platform=linux/386 alpine AS upx
RUN apk add --no-cache upx
COPY --from=build /mapupdater/mapupdater /mapupdater
RUN upx -9 /mapupdater
RUN /mapupdater version

FROM --platform=linux/386 scratch
LABEL maintainer="Bojan Cekrlic <b.cekrlic@cargox.io>"
COPY --from=upx /mapupdater /mapupdater
ENTRYPOINT [ "/mapupdater" ]

# ================ linux/amd64 ================
FROM --platform=linux/amd64 alpine AS upx
RUN apk add --no-cache upx
COPY --from=build /mapupdater/mapupdater /mapupdater
RUN upx -9 /mapupdater
RUN /mapupdater version

FROM --platform=linux/amd64 scratch
LABEL maintainer="Bojan Cekrlic <b.cekrlic@cargox.io>"
COPY --from=upx /mapupdater /mapupdater
ENTRYPOINT [ "/mapupdater" ]

# ================ linux/arm/v5 ================
FROM --platform=linux/arm/v5 scratch
LABEL maintainer="Bojan Cekrlic <b.cekrlic@cargox.io>"
COPY --from=build /mapupdater/mapupdater /mapupdater
ENTRYPOINT [ "/mapupdater" ]

# ================ linux/arm/v6 ================
FROM --platform=linux/arm/v6 scratch
LABEL maintainer="Bojan Cekrlic <b.cekrlic@cargox.io>"
COPY --from=build /mapupdater/mapupdater /mapupdater
ENTRYPOINT [ "/mapupdater" ]

# ================ linux/arm/v7 ================
FROM --platform=linux/arm/v7 scratch
LABEL maintainer="Bojan Cekrlic <b.cekrlic@cargox.io>"
COPY --from=build /mapupdater/mapupdater /mapupdater
ENTRYPOINT [ "/mapupdater" ]

# ================ linux/ppc64 ================
FROM --platform=linux/ppc64 scratch
LABEL maintainer="Bojan Cekrlic <b.cekrlic@cargox.io>"
COPY --from=build /mapupdater/mapupdater /mapupdater
ENTRYPOINT [ "/mapupdater" ]

# ================ linux/ppc64le ================
FROM --platform=linux/ppc64le scratch
LABEL maintainer="Bojan Cekrlic <b.cekrlic@cargox.io>"
COPY --from=build /mapupdater/mapupdater /mapupdater
ENTRYPOINT [ "/mapupdater" ]

# ================ linux/mips ================
FROM --platform=linux/mips scratch
LABEL maintainer="Bojan Cekrlic <b.cekrlic@cargox.io>"
COPY --from=build /mapupdater/mapupdater /mapupdater
ENTRYPOINT [ "/mapupdater" ]

# ================ linux/mipsle ================
FROM --platform=linux/mipsle scratch
LABEL maintainer="Bojan Cekrlic <b.cekrlic@cargox.io>"
COPY --from=build /mapupdater/mapupdater /mapupdater
ENTRYPOINT [ "/mapupdater" ]

# ================ linux/mips32 ================
FROM --platform=linux/mips32 scratch
LABEL maintainer="Bojan Cekrlic <b.cekrlic@cargox.io>"
COPY --from=build /mapupdater/mapupdater /mapupdater
ENTRYPOINT [ "/mapupdater" ]

# ================ linux/mips32le ================
FROM --platform=linux/mips32le scratch
LABEL maintainer="Bojan Cekrlic <b.cekrlic@cargox.io>"
COPY --from=build /mapupdater/mapupdater /mapupdater
ENTRYPOINT [ "/mapupdater" ]

# ================ linux/mips64 ================
FROM --platform=linux/mips64 scratch
LABEL maintainer="Bojan Cekrlic <b.cekrlic@cargox.io>"
COPY --from=build /mapupdater/mapupdater /mapupdater
ENTRYPOINT [ "/mapupdater" ]

# ================ linux/mips64le ================
FROM --platform=linux/mips64le scratch
LABEL maintainer="Bojan Cekrlic <b.cekrlic@cargox.io>"
COPY --from=build /mapupdater/mapupdater /mapupdater
ENTRYPOINT [ "/mapupdater" ]

# ================ linux/s390x ================
FROM --platform=linux/s390x scratch
LABEL maintainer="Bojan Cekrlic <b.cekrlic@cargox.io>"
COPY --from=build /mapupdater/mapupdater /mapupdater
ENTRYPOINT [ "/mapupdater" ]

