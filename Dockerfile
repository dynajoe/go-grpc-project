FROM golang:1.16-alpine3.13 as builder

ENV CGO_ENABLED=0
RUN apk add --no-cache git
WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -v -o /api-server ./cmd/api-server

FROM alpine:3.13

RUN apk --no-cache add ca-certificates

COPY --from=builder /api-server /api-server
CMD [ "/api-server" ]