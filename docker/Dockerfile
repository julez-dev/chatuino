FROM alpine:latest
RUN apk --no-cache add bash tzdata curl
ARG TARGETPLATFORM
COPY $TARGETPLATFORM/chatuino /usr/bin/chatuino
ENTRYPOINT ["/usr/bin/chatuino"]
