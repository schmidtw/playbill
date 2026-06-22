# playbill — minimal container image for headless NAS use.
#
# Multi-stage: build the static binary, then ship it in a tiny scratch image.
# Because the binary is CGO_ENABLED=0 / statically linked (see docs/adr/0002),
# the final image needs nothing but the binary and CA certificates (TMDB and
# Fanart.tv are reached over HTTPS). No ffmpeg, Java, or other runtime program.

# --- build stage ---------------------------------------------------------
FROM golang:1.26 AS build

WORKDIR /src

# Cache module downloads across builds when only sources change.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Stamp the version (passed by `make docker`/CI) into the binary.
ARG VERSION=docker
RUN CGO_ENABLED=0 go build -trimpath \
		-ldflags "-s -w -X main.version=${VERSION}" \
		-o /playbill ./cmd/playbill

# --- runtime stage -------------------------------------------------------
FROM scratch

# CA roots so HTTPS calls to TMDB / Fanart.tv verify.
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /playbill /playbill

# The library is mounted at /library; keys come from the environment.
#   docker run --rm -e TMDB_API_KEY -e FANARTTV_API_KEY \
#       -v /path/to/movies:/library playbill --dir /library
ENTRYPOINT ["/playbill"]
CMD ["--dir", "/library"]
