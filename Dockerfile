FROM golang:1.19
WORKDIR /build
COPY main.go go.mod /build/
COPY internal /build/internal
RUN go get
RUN CGO_ENABLED=0 go build -o run main.go

FROM scratch
COPY --from=0 /build/run /run
ENTRYPOINT ["/run"]
LABEL org.opencontainers.image.source https://github.com/galleybytes/terraform-operator-mutator
