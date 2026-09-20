# Builds all three shop_ingestor binaries into one small, non-root image.
# Deploy-time manifests select which binary to run via the container command
# (e.g. ["/app/ingestor"] vs ["/app/reconciler"] vs ["/app/migrate"]) rather
# than building three separate images for three tiny Go binaries that share
# 100% of their dependencies.

FROM golang:1.25.14-bookworm AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/ingestor ./cmd/ingestor && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/reconciler ./cmd/reconciler && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/ingestor /out/reconciler /out/migrate ./
USER nonroot:nonroot
ENTRYPOINT ["/app/ingestor"]
