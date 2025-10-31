FROM alpine:latest

ARG TARGETOS
ARG TARGETARCH

RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app/
COPY kube-watchtower_${TARGETOS}_${TARGETARCH} .
RUN chmod +x kube-watchtower_${TARGETOS}_${TARGETARCH}

ENTRYPOINT ["./kube-watchtower"]

