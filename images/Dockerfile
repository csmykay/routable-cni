FROM golang:alpine as builder

COPY . /usr/src/routable-cni

ENV HTTP_PROXY $http_proxy
ENV HTTPS_PROXY $https_proxy

WORKDIR /usr/src/routable-cni
RUN apk add --no-cache --virtual build-dependencies build-base=~0.5 && \
    make clean && \
    make build

FROM alpine:3
COPY --from=builder /usr/src/routable-cni/build/routable-cni /usr/bin/
WORKDIR /

COPY ./images/entrypoint.sh /

ENTRYPOINT ["/entrypoint.sh"]
