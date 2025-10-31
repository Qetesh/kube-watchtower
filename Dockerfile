# 构建阶段
FROM --platform=$BUILDPLATFORM golang:alpine AS builder

WORKDIR /src
COPY go.mod go.sum .
RUN go mod download

COPY . .
ARG TARGETOS TARGETARCH
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -ldflags="-s -w -X main.version=${RELEASE_TAG}" \
    -o kube-watchtower ./cmd/kube-watchtower

FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app/
COPY --from=builder /src/kube-watchtower .

ENTRYPOINT ["./kube-watchtower"]

