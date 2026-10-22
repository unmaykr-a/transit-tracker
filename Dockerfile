# Build stages run on the build host's platform and cross-compile, so multi-arch
# builds need no QEMU. Only the final stage is per target platform.

FROM --platform=$BUILDPLATFORM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY --from=web /src/web/dist ./internal/api/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
      -ldflags "-s -w -X github.com/unmaykr-a/transit-tracker/internal/buildinfo.Version=${VERSION} -X github.com/unmaykr-a/transit-tracker/internal/buildinfo.Commit=${COMMIT}" \
      -o /out/tram ./cmd/tram && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/tram /tram
# Writable data dir for the nonroot user (uid 65532). Mount a volume here to keep
# the database; with a bind mount, the host directory must be writable by the
# container user (see deploy/docker-compose.yml).
COPY --from=build --chown=65532:65532 /out/data /data
ENV TRAM_ADDR=:8080 TRAM_DATA_DIR=/data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD ["/tram", "healthcheck"]
ENTRYPOINT ["/tram"]
