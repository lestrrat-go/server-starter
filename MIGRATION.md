Migrating from v0 to v2
========================

This is a reference for moving code from the v0 line (`master`) to v2. It
covers the import path, the symbol mapping, the removed supervisor API, the
unchanged runtime contract, three deliberate behavior changes, and Windows
limitations.

## Import path

`github.com/lestrrat-go/server-starter` becomes
`github.com/lestrrat-go/server-starter/v2`. The `listener` subpackage is
gone; everything it exported now lives at the module root, in package
`starter`.

## Symbol mapping

| v0 | v2 |
|---|---|
| `listener.ListenAll()` | `starter.ListenAll()` |
| `listener.ListenPacketAll()` | `starter.ListenPacketAll()` |
| `listener.Ports()` returning `[]Listener` | `starter.Ports()` returning `List` |
| `listener.GetPortsSpecification()` | removed; read `os.Getenv(starter.PortEnvName)` |
| `listener.ServerStarterEnvVarName` | `starter.PortEnvName` |
| `listener.ErrNoListeningTarget` | `starter.ErrNoListeningTarget` |
| `listener.Listener` / `List` / `TCPListener` / `UnixListener` | same names, module root |
| — | new: `UDPListener`, `ParsePorts`, `FormatPorts`, `NewTCPListener`, `NewUDPListener`, `NewUnixListener`, `Generation`, `GenerationEnvName`, `IsUnderStartServer` |

Two things to check when you move code over:

- `ErrNoListeningTarget`'s message text changed, from `"no listening
  target"` to `"starter: no listening target"`. Code that matches on the
  string breaks; code that uses `errors.Is(err, starter.ErrNoListeningTarget)`
  keeps working.
- `Ports()` now returns the named type `List` instead of `[]Listener`. This
  still compiles for a plain assignment, and for passing the result to a
  function that takes `[]Listener`, because `List` is defined as `[]Listener`.
  It breaks two narrower usages: storing the result in a variable of a
  *different* named slice type, and taking `Ports` as a function value typed
  `func() ([]Listener, error)`.

## The supervisor is gone from the API

`Starter`, `NewStarter`, `Config`, `Run`, `Stop`, `Teardown`, `StartWorker`,
`SigFromName`, `WorkerState`, `WorkerStarted`, and `ErrFailedToStart` are all
removed from the public surface (some no longer exist, some became
internal). There is no Go replacement: a program that embedded the
supervisor should run the compiled binary as a subprocess instead.

```go
cmd := exec.CommandContext(ctx, "start_server", "--port", "8080", "--", "./myserver")
```

Worth noting: three of those symbols were never usable from outside the
module anyway. `StartWorker` took a parameter of an unexported type, so no
caller outside package `server-starter` could construct the argument.
`WorkerState`'s two values, `WorkerStarted` and `ErrFailedToStart`, were
referenced nowhere else in the codebase.

## The runtime contract is unchanged

`SERVER_STARTER_PORT`, `SERVER_STARTER_GENERATION`, and file descriptors
numbered starting at 3 all behave exactly as they did under v0. A worker
binary built against v0's `listener` package still runs correctly under a
v2 `start_server`, and a worker built against v2's `starter` package still
runs correctly under a v0 `start_server`. Only the Go import path moved.

## Three deliberate divergences from Perl's Server::Starter

The Go supervisor has always aimed to behave like the original Perl
`Server::Starter`, but v2's internal restructuring introduced three
observable differences. Each is deliberate.

1. **INT and QUIT now send `signalOnTERM` to workers, not a hardcoded
   TERM.** Perl's supervisor picks the worker signal based on which signal
   it received itself: `signal_on_term` for TERM, but a hardcoded TERM for
   INT and QUIT. In v2, the supervisor no longer inspects which signal it
   received — signal policy moved to the command layer (`start_server`
   itself), which just requests a shutdown — so all three (INT, TERM, QUIT)
   result in `signalOnTERM` being sent to the worker. This is only
   observable when an operator sets `--signal-on-term` to something other
   than TERM and then stops the supervisor with INT or QUIT rather than
   TERM.

2. **A deleted envdir file no longer leaves a stale value behind.** Under
   v0, the supervisor applied envdir values into its own environment with
   `os.Setenv` and never removed them, so deleting an envdir file left the
   last applied value in place and the worker kept seeing it. In v2, the
   worker's environment is built fresh on every spawn from the envdir
   contents plus the inherited environment, so a variable that is no
   longer present in the envdir simply falls back to whatever value it had
   in the inherited environment, instead of keeping the stale value.

3. **`KILL_OLD_DELAY`, `ENABLE_AUTO_RESTART`, and `AUTO_RESTART_INTERVAL`
   are no longer exported into the supervisor's own process environment.**
   Perl sets these in its own process and relies on `fork` to carry them
   down to the worker. A Go library must not mutate its own process-global
   environment that way, since two supervisors running in one process would
   race on it. In v2 these are passed as configuration values and the
   worker's environment is constructed explicitly from them. These
   variables are not placed in the worker's environment at all — a
   worker process never sees `KILL_OLD_DELAY`, `ENABLE_AUTO_RESTART`, or
   `AUTO_RESTART_INTERVAL`. A v0 worker did not see them either, since
   v0's supervisor never exported them for `fork` to carry down, so
   someone migrating from v0 observes no difference here. The divergence
   is from Perl's `Server::Starter`, which does export these variables
   into its own process and relies on `fork` to pass them to the worker.
   This is not a claim that the worker's environment is otherwise
   identical to v0's; other things about it have changed.

## Windows limitations

Two operations are unsupported on Windows and fail with an explicit error
rather than doing nothing silently:

- `--daemonize` fails because there is no Windows equivalent of detaching
  into a new session the way `--daemonize` does on Unix.
- `--stop` and `--restart` fail because both depend on sending a signal to
  a process by pid, which Windows does not support.
