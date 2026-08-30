# Build stage: cross-compiles a fully static binary for the target platform.
FROM --platform=$BUILDPLATFORM golang:1.27 AS build
ARG TARGETOS TARGETARCH
WORKDIR /src

# Cache module downloads between builds.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w" -o /print-service .

# Runtime stage: empty image, nothing but the static binary and CA
# certificates for the outbound HTTPS connection. Works for every target
# platform (including arm/v6 for the Pi Zero) because the binary is static.
FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /print-service /print-service

# Unprivileged runtime user.
USER 65532:65532

ENTRYPOINT ["/print-service"]
