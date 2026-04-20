FROM golang:1.25.5-alpine AS builder

WORKDIR /app

RUN apk add --no-cache ca-certificates git tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /usr/local/bin/rentflow-api .

FROM alpine:3.22

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /usr/local/bin/rentflow-api /usr/local/bin/rentflow-api

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/rentflow-api"]
