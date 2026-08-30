Migrating from v0 to v2
========================

This is a reference for moving code from the v0 line (`master`) to v2. It
covers the import path, the symbol mapping, the removed supervisor API,
runtime compatibility, the UDP extension, listener-spec validation, three
deliberate behavior changes, and Windows limitations.

## Import path

`github.com/lestrrat-go/server-starter` becomes
`github.com/lestrrat-go/server-starter/v2`. The `listener` subpackage is
gone; everything it exported now lives at the module root, in package
`starter`.

## Symbol mapping

| v0 | v2 |
|---|---|
| `listener.ListenAll()` | `starter.ListenAll()` |
| `listener.Ports()` returning `[]Listener` | `starter.Ports()` returning `List` |
| `listener.GetPortsSpecification()` | removed; read `os.Getenv(starter.PortEnvName)` |
| `listener.ServerStarterEnvVarName` | `starter.PortEnvName` |
| `listener.ErrNoListeningTarget` | `starter.ErrNoListeningTarget` |
| `listener.Listener` / `List` / `TCPListener` / `UnixListener` | same names, module root |
| — | new: `UDPListener`, `ListenPacketAll`, `ParsePorts`, `FormatPorts`, `NewTCPListener`, `NewUDPListener`, `NewUnixListener`, `Generation`, `GenerationEnvName`, `IsUnderStartServer` |

`GenerationEnvName` names the environment variable that carries the worker
generation. Use `starter.GenerationEnvName` instead of spelling
`SERVER_STARTER_GENERATION` as a string literal. `Generation` parses that
variable, while `IsUnderStartServer` checks whether it is present.

Three things to check when you move code over:

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
- `NewUnixListener` stores relative paths that overlap a TCP or UDP wire
  spelling in a canonical `./path` form. For example,
  `NewUnixListener("8080", fd)` stores `Path` as `"./8080"`, so `String` and
  `ParsePorts` keep it as a Unix listener. The prefix names the same socket in
  the current directory. Paths that already parse as Unix sockets are
  unchanged.
- `ListenAll()` accepts only TCP and unix targets, while `ListenPacketAll()`
  accepts only UDP targets. A worker with a mixed list should call `Ports()`
  and switch on `TCPListener`, `UnixListener`, and `UDPListener`. See the
  verified [`starter_mixedListeners` example](./examples/starter_mixed_listeners_example_test.go).

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

## Runtime compatibility and the UDP extension

`SERVER_STARTER_PORT`, `SERVER_STARTER_GENERATION`, and file descriptors
numbered starting at 3 retain their v0 behavior for TCP and unix sockets. A
worker binary built against v0's `listener` package still handles those
targets under a v2 `start_server`, and a worker built against v2's `starter`
package handles them under a v0 `start_server`.

UDP targets are a v2 extension. V0 workers support TCP and Unix listeners,
but not UDP listeners, so a v2 `start_server` configured with a UDP target
requires a worker built against v2's `starter` package. Their canonical
command-line and
`SERVER_STARTER_PORT` spelling is `udp://PORT`, `udp://host:PORT`, or
`udp://[ipv6]:PORT`. The v2 parser still reads unambiguous legacy forms such
as `uPORT`, `host:uPORT`, and `u[ipv6]:PORT`. A leading `u` on an otherwise
valid hostname is no longer treated as a transport marker, so
`ubuntu.internal:8080` is TCP. In the legacy `host:uPORT` form, the suffix is
the marker, so `ubuntu.internal:u8080` preserves the full hostname. Spell new
UDP targets with `udp://`.

## Listener-spec validation is stricter

V0 joined each listener's `String()` result and passed it to the worker as
`SERVER_STARTER_PORT` without validation. The public `FormatPorts` function
now rejects values that cannot be encoded as one environment variable and
read back as the same listener. This includes NUL bytes, TCP/UDP addresses or
Unix socket paths containing `;` or `=`, ambiguous Unix paths stored by
bypassing or modifying `NewUnixListener`, and other malformed listener values.

The supervisor's `Config` accepts only string port and path metadata, so
struct-literal listeners do not enter that API. Before acquiring its pid file
or binding a listener, the supervisor applies the same public `FormatPorts`
rule used to build worker metadata for every fully specified address. On Linux,
an empty Unix path is bound first and its kernel-generated address is then
validated. A NUL-prefixed Linux abstract address is converted to the
environment-safe `@` spelling before validation. Other invalid listener names
fail synchronously during `Run`. Valid TCP and unix listener specs keep the v0
wire format described above, while UDP targets use the explicit `udp://`
extension.

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
   race on it. In v2, values derived from command-line flags are passed as
   configuration instead, so flag-derived values alone do not add these
   variables to the worker's environment. Existing environment values keep
   their normal inheritance: a value present in the ambient environment
   reaches the worker, and an envdir value is overlaid on the ambient value.
   A worker can therefore see any of these variables when its launch
   environment or envdir supplies them. This matches v0's inheritance
   behavior; the divergence is from Perl's `Server::Starter`, which exports
   its configuration values before `fork` passes them to the worker.

## Windows limitations

Two operations are unsupported on Windows and fail with an explicit error
rather than doing nothing silently:

- `--daemonize` fails because there is no Windows equivalent of detaching
  into a new session the way `--daemonize` does on Unix.
- `--stop` and `--restart` fail because both depend on sending a signal to
  a process by pid, which Windows does not support.
