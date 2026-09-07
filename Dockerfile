FROM golang:1.25-bookworm AS builder

WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends curl ca-certificates git \
    && rm -rf /var/lib/apt/lists/*

# Download and install Tailwind CLI for Linux
RUN curl -sL "https://github.com/tailwindlabs/tailwindcss/releases/download/v4.3.0/tailwindcss-linux-x64" -o tailwindcss \
    && chmod +x tailwindcss

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go install github.com/a-h/templ/cmd/templ@v0.3.1001
RUN templ generate
RUN ./tailwindcss -i ./styles/input.css -o ./public/css/styles.css --minify
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o hylete ./cmd/main.go

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates libgcc-s1 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /app/hylete .
COPY --from=builder /app/public ./public

EXPOSE 8080
ENTRYPOINT ["/app/hylete"]
