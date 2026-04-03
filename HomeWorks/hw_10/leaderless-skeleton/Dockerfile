FROM golang:1.22-alpine

WORKDIR /app

COPY . .

RUN go mod tidy

RUN go build -o kv-node main.go

EXPOSE 8080

CMD ["./kv-node"]