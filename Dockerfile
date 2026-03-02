FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o /bin/api-server ./api/main.go

FROM alpine:latest

RUN apk add --no-cache nodejs npm

WORKDIR /app

COPY --from=builder /bin/api-server /usr/local/bin/api-server

# COPY anchor-bot/ ./anchor-bot/
# RUN cd anchor-bot && npm install
# RUN mkdir -p /app/anchor-bot/logs

EXPOSE 2999

CMD ["api-server"]