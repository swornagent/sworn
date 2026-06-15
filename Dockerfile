# Multi-stage static build for SwornAgent CLI.
# Produces a minimal scratch image with a single sworn binary.

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION:-0.0.0-dev}" -trimpath -o /sworn ./cmd/sworn

FROM scratch
COPY --from=build /sworn /sworn
ENTRYPOINT ["/sworn"]