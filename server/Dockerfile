FROM golang:1.27-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o listune-server .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates chromaprint ffmpeg
COPY --from=builder /app/listune-server /usr/local/bin/
EXPOSE 8090
ENTRYPOINT ["listune-server"]
