# ---------- build ----------
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Binário estático (o driver SQLite usado é Go puro, sem CGO)
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app .

# ---------- runtime ----------
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
EXPOSE 8082
USER nonroot
ENTRYPOINT ["/app"]
