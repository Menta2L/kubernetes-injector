FROM golang:1.20 as build
RUN go install golang.org/x/lint/golint@latest
WORKDIR /build
COPY . ./
RUN make release

FROM scratch
WORKDIR /
COPY --from=build /build/k8-injector /

ENTRYPOINT ["/k8-injector"]
