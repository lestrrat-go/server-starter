FROM golang:1.27-bookworm

RUN go install github.com/tcnksm/ghr@latest

COPY build-server_starter.sh release-server_starter.sh /

VOLUME ["/work/src/github.com/lestrrat-go/server-starter", "/work/artifacts"]

CMD ["go", "version"]