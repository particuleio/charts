ARG BASE_VERSION=latest

FROM gcr.io/istio-release/base:${BASE_VERSION}

COPY client /usr/local/bin/client
COPY server /usr/local/bin/server
COPY certs/cert.crt /cert.crt
COPY certs/cert.key /cert.key

ENTRYPOINT ["/usr/local/bin/server"]
