FROM golang:1.24 AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN go build -o ./goapp ./app

FROM golang:1.24 AS runner

COPY --from=builder /app/goapp /app/goapp
COPY .env .env

EXPOSE 8080

CMD ["/app/goapp"]