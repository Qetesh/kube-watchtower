FROM alpine:latest

ARG TARGETARCH

RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app/
COPY kube-watchtower_${TARGETARCH} ./kube-watchtower
RUN chmod +x kube-watchtower_${TARGETARCH}

ENTRYPOINT ["./kube-watchtower"]

