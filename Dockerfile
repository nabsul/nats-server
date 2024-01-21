FROM golang:1.21-alpine AS builder


WORKDIR /go/src/app
COPY go.mod /go/src/app/
COPY go.sum /go/src/app/
RUN go mod download

COPY . .
RUN go build -o /go/bin/app

FROM alpine:3.19

RUN addgroup -S -g 1001 appgroup && adduser -S -u 1001 -G appgroup appuser
# RUN apk add --update ca-certificates
USER appuser

COPY --from=builder /go/bin/app /bin/nats-server

EXPOSE 4222 8222 6222 5222

ENTRYPOINT ["/bin/nats-server"]
