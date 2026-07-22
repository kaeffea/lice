# syntax=docker/dockerfile:1.7

FROM golang:1.26.5-bookworm AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/lice-api ./cmd/lice-api && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/lice-admin ./cmd/lice-admin

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/lice-api /usr/local/bin/lice-api
COPY --from=build /out/lice-admin /usr/local/bin/lice-admin
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/lice-api"]
