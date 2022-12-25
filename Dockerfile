# Use the official Golang image to create a build artifact.
# This is based on Debian and sets the GOPATH to /go.
# https://hub.docker.com/_/golang
FROM golang:1.19-buster as builder

# Create and change to the app directory.
WORKDIR /go/src/cloudrun/workdir

# Retrieve application dependencies.
# This allows the container build to reuse cached dependencies.
COPY go.* ./
RUN go mod download

# Copy local code to the container image.
COPY . ./

# Build the command inside the container.
RUN go build -v -o music-suka-yoga

# Use the official Debian slim image for a lean production container.
# https://hub.docker.com/_/debian
# https://docs.docker.com/develop/develop-images/multistage-build/#use-multi-stage-builds
FROM debian:buster-slim
RUN set -x && apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y \
    ca-certificates && \
    rm -rf /var/lib/apt/lists/*

# Copy the binary to the production image from the builder stage.
COPY --from=builder /go/src/cloudrun/workdir/music-suka-yoga /music-suka-yoga

# set env vars
# DON'T !!!

# Copy templates, static and such
COPY templates templates/
COPY static static/
# COPY favicon.ico .
COPY robots.txt .
# For firebase (not really picked up in CloudRun)
COPY .firebase-credentials.json ./
# Project service account
COPY .music-runner.json ./

# Run the web service on container startup.
CMD ["/music-suka-yoga"]