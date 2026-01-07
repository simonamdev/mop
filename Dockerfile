FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /app/mop .

FROM scratch

WORKDIR /app

COPY --from=builder /app/mop .

CMD ["./mop"]
