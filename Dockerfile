# One image: refract + tectonic + synctex + git + ripgrep + bubblewrap, non-root, data in /data.
# It runs as REFRACT_PROFILE=personal by default; REFRACT_PROFILE=team isolates
# compiles with bubblewrap (see "Run it for a team" in the README).
# Build for one platform with `docker build .`, or both with `make docker`.

# --- frontend (platform-independent output, built natively) ---
FROM --platform=$BUILDPLATFORM node:24.22.3-alpine3.24 AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# --- Go binary (cross-compiled, no CGO) ---
FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine3.24 AS go
ARG TARGETOS TARGETARCH
# Shown by `refract version`; the release workflow passes the tag.
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd cmd
COPY internal internal
COPY web/embed.go web/
COPY --from=web /src/web/dist web/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/refract ./cmd/refract

# --- synctex CLI from the official parser sources (Tectonic ships none) ---
# Pins (commit, zlib) are in packaging/tools.lock, shared with CI and the release;
# scripts/build-synctex.sh compiles synctex with the pinned zlib statically.
FROM alpine:3.24.4 AS synctex
RUN apk upgrade --no-cache && apk add --no-cache build-base git curl
COPY packaging/tools.lock /src/packaging/tools.lock
COPY scripts/tools-common.sh scripts/build-synctex.sh /src/scripts/
RUN /src/scripts/build-synctex.sh linux "$(case $(uname -m) in x86_64) echo amd64 ;; *) echo arm64 ;; esac)" /out/synctex

# --- Tectonic, pinned official static build, checksum verified (packaging/tools.lock) ---
FROM --platform=$BUILDPLATFORM alpine:3.24.4 AS tectonic
ARG TARGETARCH
ENV TOOLS_ROOT=/src
RUN apk add --no-cache curl
COPY packaging/tools.lock /src/packaging/tools.lock
COPY scripts/tools-common.sh /src/scripts/
RUN . /src/scripts/tools-common.sh \
 && asset=$(tectonic_asset linux "$TARGETARCH") \
 && fetch "https://github.com/tectonic-typesetting/tectonic/releases/download/tectonic%40${TECTONIC_VERSION}/tectonic-${TECTONIC_VERSION}-${asset%|*}.${asset#*|}" \
      "$(lock_get TECTONIC_SHA256 linux "$TARGETARCH")" /tmp/t.tgz \
 && tar -xzf /tmp/t.tgz -C /usr/local/bin tectonic

# --- runtime ---
FROM alpine:3.24.4
# bubblewrap and prlimit (util-linux-misc) run the compile sandbox of the team profile.
# apk upgrade takes the release branch's security fixes published since the base image.
RUN apk upgrade --no-cache \
 && apk add --no-cache ca-certificates git ripgrep bubblewrap util-linux-misc \
 && adduser -D -u 10001 refract \
 && mkdir /data /opt/refract /opt/refract/tex && chown refract:refract /data /opt/refract/tex
COPY --from=tectonic /usr/local/bin/tectonic /usr/local/bin/
COPY --from=synctex /out/synctex /usr/local/bin/
COPY --from=go /out/refract /usr/local/bin/
# Prepares the offline TeX bundle the sandbox needs, into a volume at /opt/refract/tex.
COPY packaging/server/prepare-bundle.sh /usr/local/bin/refract-prepare-bundle
# The bundle is pinned for reproducible builds (the relay URL is a moving redirect).
# Every interface inside the container: -p decides the mapping on the Docker host
# (containers on a shared Docker network can still connect).
ENV REFRACT_ADDR=:8080 \
    REFRACT_DATA_DIR=/data \
    TECTONIC_CACHE_DIR=/data/tectonic-cache \
    REFRACT_TECTONIC_BUNDLE=https://data1b.fullyjustified.net/tlextras-2022.0r0.tar
USER refract
VOLUME /data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["refract"]
