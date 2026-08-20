# Building custom health checker
FROM golang:1.26.6-trixie@sha256:b75d466dd608587fd66cca705a307ba65b889827d06ad61d6a75f0482b51b7c7 AS health-build-env


# Copying source
WORKDIR /go/src/app
COPY ./healthcheck/go.mod ./healthcheck/go.sum* ./
RUN go mod download
COPY ./healthcheck /go/src/app

# Compiling
RUN CGO_ENABLED=0 go build -o /go/bin/healthchecker

# Building bouncer
FROM golang:1.26.6-trixie@sha256:b75d466dd608587fd66cca705a307ba65b889827d06ad61d6a75f0482b51b7c7 AS build-env


# Copying source
WORKDIR /go/src/app
COPY go.mod go.sum ./
RUN go mod download
COPY . /go/src/app

# Compiling
RUN CGO_ENABLED=0 go build -o /go/bin/app

FROM gcr.io/distroless/static:nonroot@sha256:f7f8f729987ad0fdf6b05eeeae94b26e6a0f613bdf46feea7fc40f7bd72953e6
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
