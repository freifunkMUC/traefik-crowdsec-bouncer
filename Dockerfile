# Both Go stages run on the build machine and cross-compile for the target
# platform: the binaries are static (CGO_ENABLED=0), so nothing in them needs
# the target's userland, and building under QEMU took the multi-platform
# publish the better part of an hour.

# Building custom health checker
FROM --platform=$BUILDPLATFORM golang:1.27.1-trixie@sha256:3b77fc618ec235a1ab412de7737f120dd507c57e8d87de4cbb7994fb94275ed5 AS health-build-env
ARG TARGETOS TARGETARCH TARGETVARIANT

# Copying source
WORKDIR /go/src/app
COPY ./healthcheck/go.mod ./healthcheck/go.sum* ./
RUN go mod download
COPY ./healthcheck /go/src/app

# Compiling; GOARM is only read for 32-bit arm, where the variant is v7
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=${TARGETVARIANT#v} \
    go build -o /go/bin/healthchecker

# Building bouncer
FROM --platform=$BUILDPLATFORM golang:1.27.1-trixie@sha256:3b77fc618ec235a1ab412de7737f120dd507c57e8d87de4cbb7994fb94275ed5 AS build-env
ARG TARGETOS TARGETARCH TARGETVARIANT

# Copying source
WORKDIR /go/src/app
COPY go.mod go.sum ./
RUN go mod download
COPY . /go/src/app

# Compiling
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=${TARGETVARIANT#v} \
    go build -o /go/bin/app

FROM gcr.io/distroless/static:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3
COPY --from=health-build-env --chown=nonroot:nonroot /go/bin/healthchecker /
COPY --from=build-env --chown=nonroot:nonroot /go/bin/app /

# Sensible production default; override with -e GIN_MODE=debug for verbose
# local troubleshooting. Without this, the image runs in Gin's debug mode
# (extra log noise, a startup warning) unless the deployer sets it themselves
# -- none of the example compose files in this repo did.
ENV GIN_MODE=release

# Run as a non root user.
USER nonroot

# Using custom health checker
HEALTHCHECK --interval=10s --timeout=5s --retries=2\
  CMD ["/healthchecker"]

# Run app
CMD ["/app"]
