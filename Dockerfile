# 构建阶段
FROM golang:alpine AS builder

WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=$(go env GOARCH) go build -a -installsuffix cgo -o kube-watchtower ./cmd/kube-watchtower

FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata
WORKDIR /root/
COPY --from=builder /app/kube-watchtower .

ARG RELEASE_TAG
ENV RELEASE_TAG=${RELEASE_TAG}

ENTRYPOINT ["./kube-watchtower"]

