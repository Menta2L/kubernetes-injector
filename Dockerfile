FROM golang:1.20 as build
ARG KUBECTL_VERSION=1.22.0

RUN go install golang.org/x/lint/golint@latest
WORKDIR /build
COPY . ./
# Download kubectl CLI
RUN curl -LO https://dl.k8s.io/release/v"${KUBECTL_VERSION}"/bin/linux/amd64/kubectl && \
    chmod +x kubectl
RUN make release

FROM alpine:3.14

RUN apk add -u shadow libc6-compat curl openssl && \
    rm -rf /var/cache/apk/*

# Add Limited user
RUN groupadd -r kubernetes-injector \
             -g 777 && \
    useradd -c "kubernetes-injector runner account" \
            -g kubernetes-injector \
            -u 777 \
            -m \
            -r \
            kubernetes-injector

USER kubernetes-injector
WORKDIR /
COPY --from=build /build/k8-injector \
     /build/kubectl \
     /usr/local/bin/
ENTRYPOINT ["//usr/local/bin/k8-injector"]
