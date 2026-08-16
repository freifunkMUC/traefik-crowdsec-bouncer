# Building custom health checker
FROM golang:1.26.4-trixie@sha256:68b7145ec43d1820b9a56704554b53d1520aa2a15cb5233e374188a31b2a1bce AS health-build-env


# Copying source
WORKDIR /go/src/app
COPY ./healthcheck/go.mod ./healthcheck/go.sum* ./
RUN go mod download
COPY ./healthcheck /go/src/app

# Compiling
RUN CGO_ENABLED=0 go build -o /go/bin/healthchecker

# Building bouncer
FROM golang:1.26.4-trixie@sha256:68b7145ec43d1820b9a56704554b53d1520aa2a15cb5233e374188a31b2a1bce AS build-env


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

# Run as a non root user.
USER nonroot

# Using custom health checker
HEALTHCHECK --interval=10s --timeout=5s --retries=2\
  CMD ["/healthchecker"]

# Run app
CMD ["/app"]
