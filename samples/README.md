# Sample workers

Each directory contains an independent Go module and a `go.work` file that
uses the checked-out `server-starter` module. Build the workers from the
repository root:

```sh
go build -C ./samples/http
go build -C ./samples/udp-echo
go build -o ./start_server ./cmd/start_server
```

Run the HTTP worker on an inherited TCP listener:

```sh
./start_server --port=127.0.0.1:8080 -- ./samples/http/http
```

Run the UDP echo worker on an inherited UDP packet connection:

```sh
./start_server --port=127.0.0.1:u9000 -- ./samples/udp-echo/udp-echo
```

The workers require `start_server`; they do not bind their own ports. Send
`SIGTERM` to `start_server` to stop its active worker cleanly.
