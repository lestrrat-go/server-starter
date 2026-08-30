# Incompatible Changes from v0 to v2

These are the changes in v2. For a step-by-step migration guide with symbol
mappings and code examples, see [MIGRATION.md](./MIGRATION.md).

## Module and public API

* The module path is now `github.com/lestrrat-go/server-starter/v2`.

* The `listener` subpackage is folded into the module root, package `starter`.
  Import `github.com/lestrrat-go/server-starter/v2` instead.

* The supervisor is no longer importable as a Go library. It ships only as the
  `start_server` command. Programs that embedded the supervisor should run the
  compiled binary as a subprocess instead.

* New worker-facing helpers are available: `ParsePorts`, `NewTCPListener`,
  `NewUDPListener`, `NewUnixListener`, `Generation`, and `IsUnderStartServer`.
  `UDPListener` and `ListenPacketAll` provide typed UDP support for workers
  started by this implementation. `ListenAll` accepts TCP and Unix targets
  only, while `ListenPacketAll` accepts UDP targets only. Programs with mixed
  listeners should use `Ports` and the exported concrete listener types.

## Sample workers

* The [`samples/`](./samples/README.md) directory provides runnable HTTP and
  UDP echo workers. They demonstrate stream and packet listener recovery from
  `start_server`.

## Listener specifications

* TCP and Unix listener specifications retain Server::Starter's
  `SERVER_STARTER_PORT` representation: entries are `target=fd` values
  separated by `;`. The following Server::Starter-compatible specification is
  accepted:

  ```sh
  start_server --port 127.0.0.1:8080 --path /run/example.sock -- ./worker
  ```

  A target or path cannot contain `;` or `=`, because Server::Starter reserves
  those characters in the inherited environment value. These values cannot be
  represented by either implementation:

  ```sh
  start_server --port 'api;blue:8080' -- ./worker
  start_server --path '/run/example=old.sock' -- ./worker
  ```

* On Linux, an empty Unix path uses a kernel-generated abstract address. The
  address reaches workers in the environment-safe `@` spelling.

* UDP targets use Server::Starter's `uPORT` or `host:uPORT` syntax. The
  inherited `SERVER_STARTER_PORT` value retains the same `target=fd` form as
  TCP, so Perl workers receive the same descriptors and ignore the added
  metadata. This implementation sets `LSS2_SOCKET_TYPES` as `fd=type` entries
  so its workers can identify UDP descriptors. A hostname that begins with
  `u` remains TCP unless the port itself begins with `u`.

* `NewUnixListener` and `UnixListener.String` prefix relative Unix paths that
  look like TCP or UDP targets with `./`. This preserves their listener type
  across `SERVER_STARTER_PORT` round trips.

For example, this Server::Starter-compatible command starts a worker with TCP,
UDP, and Unix listeners:

```sh
start_server \
  --port 127.0.0.1:8080 \
  --port 127.0.0.1:u5353 \
  --path /run/example.sock \
  -- ./worker
```

For a relative Unix socket named `8080`, use `--path ./8080` so it is not
interpreted as a TCP target.

## Command behavior

* `start_server` rejects unknown `--signal-on-hup` and `--signal-on-term`
  names instead of silently using `TERM`. `XFSZ` is recognized correctly.

## Deliberate behavior changes from Perl Server::Starter

* When receiving `INT` or `QUIT`, the supervisor sends the worker the signal
  configured with `--signal-on-term`, as it does for `TERM`, rather than a
  hardcoded `TERM`. This affects only a non-`TERM` `--signal-on-term` setting.

* When a file is deleted from the envdir, its environment variable falls back
  to the inherited environment instead of retaining the last envdir value.
  The worker environment is built fresh on every spawn.

* `KILL_OLD_DELAY`, `ENABLE_AUTO_RESTART`, and `AUTO_RESTART_INTERVAL` are no
  longer exported into the supervisor's own environment. Command-line values
  are passed as configuration, while ambient and envdir values retain normal
  worker inheritance. Envdir values take precedence.
