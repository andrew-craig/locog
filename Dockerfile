FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN apk add --no-cache gcc musl-dev sqlite-dev
RUN CGO_ENABLED=1 GOOS=linux go build -o logservice ./cmd/logservice

FROM alpine:latest
RUN apk --no-cache add ca-certificates sqlite wget

WORKDIR /app
COPY --from=builder /app/logservice .

EXPOSE 5081
ENTRYPOINT ["./logservice"]
