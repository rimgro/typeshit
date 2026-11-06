FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/typeshit ./cmd/server

FROM alpine:3.20
RUN adduser -D -u 10001 app
WORKDIR /app
COPY --from=build /out/typeshit /app/typeshit
USER app
VOLUME ["/app/data"]
EXPOSE 8080
ENTRYPOINT ["/app/typeshit", "-addr", "0.0.0.0:8080", "-data", "/app/data"]
