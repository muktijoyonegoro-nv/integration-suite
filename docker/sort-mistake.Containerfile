# Multi-stage Containerfile for sort-mistake runtime
FROM golang:1.24-alpine AS builder

RUN apk --no-cache add build-base librdkafka-dev pkgconfig

WORKDIR /src
COPY . .

RUN go build -mod=vendor -tags musl -o /app/sort-mistake .

FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata librdkafka

WORKDIR /app

COPY --from=builder /app/sort-mistake /app/sort-mistake
COPY qa.env.yaml /app/qa.env.yaml

EXPOSE 9000

ENTRYPOINT ["/app/sort-mistake"]
CMD ["/app/qa.env.yaml"]

