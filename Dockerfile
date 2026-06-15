# Copyright 2026 Query Farm LLC - https://query.farm
#
# Container for the easter VGI worker's HTTP transport. DuckDB clients ATTACH it
# over http:// — see README.

# ---- Build stage -----------------------------------------------------------
FROM golang:1.25-bookworm AS build

WORKDIR /src

# Cache module downloads first.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build a static binary (CGO disabled — pure-Go deps only) so it runs on a
# minimal runtime image. VGI_EASTER_GIT_COMMIT (if passed) is stamped as the
# catalog's implementation_version.
ARG VGI_EASTER_GIT_COMMIT=""
RUN CGO_ENABLED=0 GOFLAGS=-trimpath \
    go build -ldflags "-s -w -X main.packageVersion=${VGI_EASTER_GIT_COMMIT}" \
    -o /out/vgi-easter-go .

# ---- Runtime stage ---------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/vgi-easter-go /usr/local/bin/vgi-easter-go

# Fly routes external traffic to this internal port (see fly.toml).
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/vgi-easter-go"]
CMD ["--http", "--addr", "0.0.0.0:8080", "--log-format", "json"]
