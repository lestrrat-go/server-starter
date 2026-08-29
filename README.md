server-starter
=================

Go port of ```start_server``` utility (a.k.a. [Server::Starter](https://metacpan.org/pod/Server::Starter)).

[![CI](https://github.com/lestrrat-go/server-starter/actions/workflows/ci.yml/badge.svg?branch=develop/v2)](https://github.com/lestrrat-go/server-starter/actions/workflows/ci.yml)

## DESCRIPTION

*note: this description is almost entirely taken from the original Server::Starter module*

The ```start_server``` utility is a superdaemon for hot-deploying server programs.

It is often a pain to write a server program that supports graceful restarts, with no resource leaks. Server::Starter solves the problem by splitting the task into two: ```start_server``` works as a superdaemon that binds to zero or more TCP ports or unix sockets, and repeatedly spawns the server program that actually handles the necessary tasks (for example, responding to incoming connections). The spawned server programs under ```start_server``` call accept(2) and handle the requests.

To gracefully restart the server program, send SIGHUP to the superdaemon. The
superdaemon spawns a new server program, and if (and only if) it starts up
successfully, sends the configured restart or termination signal to the old
server program (`SIGTERM` by default).

By using ```start_server``` it is much easier to write a hot-deployable server. Following are the only requirements a server program to be run under ```start_server``` should conform to:

- receive file descriptors to listen to through an environment variable
- perform a graceful shutdown when receiving the configured termination signal (`SIGTERM` by default)

When `--daemonize` is used on Unix, the launching process waits until the first
worker passes its startup check. Startup failures are returned to the launcher,
and the daemon shuts down its worker before reporting cancellation or a lost
readiness connection. The regular foreground supervisor keeps retrying workers
that fail to start.

On Unix, `--stop` and `--restart` attribute the state-file PID to the live lock
owner for new record-locked supervisors and Linux legacy flock-only
supervisors. Linux legacy flock-only control requires readable `/proc/locks`;
if that file is unavailable or cannot be read, control fails safely without
signaling the recorded PID. On other Unix systems, legacy flock-only control
verifies only that the lock is held and the recorded process is live, then
waits for that validated process to exit. Store the PID file in a root-owned or
mode-0700 directory that another user cannot replace, and ensure no traversed
ancestor lets another user replace the next path entry, especially when
controlling a legacy supervisor on those systems.

Many PSGI servers support this. If you want your Go program to support it,
import the root of this module (`github.com/lestrrat-go/server-starter/v2`, see
the [package docs](https://github.com/lestrrat-go/server-starter/tree/develop/v2))
for the worker-side implementation, which turns inherited descriptors into
`net.Listener` and `net.PacketConn` values.

## INSTALLATION

The v2 module is not tagged yet. Install the current v2 revision explicitly:

```
go install github.com/lestrrat-go/server-starter/v2/cmd/start_server@v2.0.0-20260828032310-12627c1634fe
```

Replace the pinned version with `@latest` after v2 is tagged.

`start_server` is a binary, not a Go library: you install and run it, you
do not import it. The Go module `github.com/lestrrat-go/server-starter/v2`
gives you only the worker-facing side of the protocol, described below.

Coming from the v0 line of this module (the `listener` subpackage and the
importable supervisor)? See [MIGRATION.md](./MIGRATION.md).

## WORKER-SIDE USAGE

A program that runs under `start_server` checks whether it was launched
that way, recovers the listening sockets the supervisor already bound, and
serves on them instead of opening its own listener:

```go
if !starter.IsUnderStartServer() {
	log.Fatal("this program must be run under start_server")
}

listeners, err := starter.ListenAll()
if err != nil {
	log.Fatal(err)
}

var wg sync.WaitGroup
for _, listener := range listeners {
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := http.Serve(listener, handler); err != nil {
			log.Printf("failed to serve %s: %s", listener.Addr(), err)
		}
	}()
}
wg.Wait()
```

`ListenAll` expects every inherited endpoint to be TCP or unix.
`ListenPacketAll` expects every endpoint to be UDP. For a mixed list, call
`Ports` and switch on the exported `TCPListener`, `UnixListener`, and
`UDPListener` types: use `ListenPacket` for UDP and `Listen` for the stream
listener types. The verified
[`starter_mixedListeners` example](./examples/starter_mixed_listeners_example_test.go)
shows that workflow.

See the `examples/` directory for tested examples of the worker-side APIs,
including the mixed-list workflow.

## SUPERVISOR PORT DESCRIPTORS

The `--port` option accepts an optional `=fd` suffix when an integration
requires a specific inherited descriptor. Explicit descriptors must be from 3
through 1024. The complete listener layout may leave at most 256 unused
descriptor slots, which the supervisor fills before starting each worker.

## WINDOWS

`start_server` runs on Windows, with two limitations: `--daemonize` is not
supported, and `--stop` / `--restart` are not supported because Windows has
no way to deliver a signal to a process by pid. All three fail with an
explicit error rather than doing nothing silently.
