FROM golang:alpine AS builder

WORKDIR /app
COPY go.mod ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -a ./cmd/worker

FROM alpine:latest

WORKDIR /app

COPY /containers/app/docker-entrypoint.sh ./docker-entrypoint.sh
RUN chmod +x ./docker-entrypoint.sh

COPY --from=builder /app/server ./
COPY --from=builder /app/worker ./

EXPOSE 8080

ENTRYPOINT ["/app/docker-entrypoint.sh"]