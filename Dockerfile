FROM golang:1.22

WORKDIR /app

COPY . .

RUN go mod download

RUN go build -o node-brainer ./cmd/main.go

# CMD ["./node-brainer download --eth-client geth"]
