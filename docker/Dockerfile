FROM alpine:latest
RUN apk --no-cache add bash tzdata curl
COPY chatuino /usr/bin/chatuino
ENTRYPOINT ["/usr/bin/chatuino"]
