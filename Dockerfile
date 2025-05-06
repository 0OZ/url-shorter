FROM golang:1.24 AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN go build -o ./goapp ./app

FROM golang:1.24 AS runner

# Install PostgreSQL client for the wait script
RUN apt-get update && apt-get install -y postgresql-client && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /app/goapp /app/goapp
COPY app/templates /app/templates
COPY app/wait-for-postgres.sh /app/wait-for-postgres.sh
COPY .env .env

# Ensure the script is executable
RUN chmod +x /app/wait-for-postgres.sh

EXPOSE 8080

# Use the wait script before starting the application
CMD ["/app/wait-for-postgres.sh", "/app/goapp"]