# syntax=docker/dockerfile:1
# Build: podman build -t localhost/ff-cripto:latest .

ARG GO_VERSION=1.24

# ---- build ----
FROM docker.io/library/golang:${GO_VERSION} AS build
WORKDIR /src

# Baixa dependências numa camada separada (cache entre builds)
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# CGO_ENABLED=0 -> binário estático, roda em imagem sem libc
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

# ---- runtime ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api /api
USER nonroot:nonroot
ENTRYPOINT ["/api"]
