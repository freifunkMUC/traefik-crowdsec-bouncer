# Building custom health checker
FROM golang:1.27rc2-trixie@sha256:2db0e0e18bbc0433b75a534f988865a860c7f91198c3953acf602f128cd23b6d AS health-build-env


# Copying source
WORKDIR /go/src/app
COPY ./healthcheck/go.mod ./healthcheck/go.sum* ./
RUN go mod download
COPY ./healthcheck /go/src/app

# Compiling
RUN CGO_ENABLED=0 go build -o /go/bin/healthchecker

# Building bouncer
FROM golang:1.27rc2-trixie@sha256:2db0e0e18bbc0433b75a534f988865a860c7f91198c3953acf602f128cd23b6d AS build-env


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
