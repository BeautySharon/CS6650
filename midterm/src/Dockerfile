# ---------- build stage ----------
FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY . .

RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o server .

# ---------- run stage ----------
FROM alpine:latest

WORKDIR /root/
COPY --from=builder /app/server .

EXPOSE 8080
CMD ["./server"]