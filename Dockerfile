FROM golang:1.25 AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /platform ./cmd/platform

FROM gcr.io/distroless/static-debian12

COPY --from=builder /platform /platform

EXPOSE 8080 8081 8083

ENTRYPOINT ["/platform"]
