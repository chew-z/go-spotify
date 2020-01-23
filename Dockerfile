# Use the official Golang image to create a build artifact.
# This is based on Debian and sets the GOPATH to /go.
# https://hub.docker.com/_/golang
# FROM golang:1.13 as builder
FROM golang:1.13-buster as builder

# Create and change to the app directory.
WORKDIR /go/src/cloudrun/spotify

# Retrieve application dependencies.
# This allows the container build to reuse cached dependencies.
COPY go.* .
RUN go mod download

# Copy local code to the container image.
COPY . .

# Build the command inside the container.
RUN CGO_ENABLED=0 GOOS=linux go build -v -o go-spotify

# Use a Docker multi-stage build to create a lean production image.
# https://docs.docker.com/develop/develop-images/multistage-build/#use-multi-stage-builds
# Use Google managed base image
# https://cloud.google.com/container-registry/docs/managed-base-images
# FROM marketplace.gcr.io/google/debian9:latest
# or Google distroless images = 'precisely what's necessary for your app'
# https://github.com/GoogleContainerTools/distroless debian buster for go1.13
FROM gcr.io/distroless/base-debian10

# Copy the binary to the production image from the builder stage.
# COPY --from=builder /go/src/cloudrun/spotify/go-spotify /go-spotify
COPY --from=builder /go/src/cloudrun/spotify/go-spotify /go-spotify

# set env vars
# DON'T !!!

# Copy templates, static and such
COPY templates templates/
COPY static static/
COPY favicon.ico .
COPY robots.txt .
# For firebase (not really picked up in CloudRun)
COPY .firebase-credentials.json ./
# Project service account
COPY .go-spotify.json ./

# Run the web service on container startup.
CMD ["/go-spotify"]