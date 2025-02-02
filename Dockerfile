# Building custom health checker
FROM golang:1.23.4-bookworm AS health-build-env

# Copying source
WORKDIR /go/src/app
COPY ./healthcheck /go/src/app

# Installing dependencies
RUN go get -d -v ./...

# Compiling
RUN go build -o /go/bin/healthchecker

# Building bouncer
FROM golang:1.23.4-bookworm AS build-env

# Copying source
WORKDIR /go/src/app
COPY . /go/src/app

# Installing dependencies
RUN go get -d -v ./...

# Compiling
RUN go build -o /go/bin/app

FROM gcr.io/distroless/base:nonroot
COPY --from=health-build-env --chown=nonroot:nonroot /go/bin/healthchecker /
COPY --from=build-env --chown=nonroot:nonroot /go/bin/app /

# Run as a non root user.
USER nonroot

# Using custom health checker
HEALTHCHECK --interval=10s --timeout=5s --retries=2\
  CMD ["/healthchecker"]

# Run app
CMD ["/app"]
