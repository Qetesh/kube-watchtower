# 构建阶段
FROM golang:alpine AS builder

WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=$(go env GOARCH) go build -a -installsuffix cgo -o kubewatchtower ./cmd/kubewatchtower

FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata
WORKDIR /root/
COPY --from=builder /app/kubewatchtower .

ENTRYPOINT ["./kubewatchtower"]

