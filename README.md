server-starter
=================

Go port of ```start_server``` utility (a.k.a. [Server::Starter](https://metacpan.org/pod/Server::Starter)).

[![CI](https://github.com/lestrrat-go/server-starter/actions/workflows/ci.yml/badge.svg?branch=develop/v2)](https://github.com/lestrrat-go/server-starter/actions/workflows/ci.yml)

## DESCRIPTION

*note: this description is almost entirely taken from the original Server::Starter module*

The ```start_server``` utility is a superdaemon for hot-deploying server programs.

It is often a pain to write a server program that supports graceful restarts, with no resource leaks. Server::Starter solves the problem by splitting the task into two: ```start_server``` works as a superdaemon that binds to zero or more TCP ports or unix sockets, and repeatedly spawns the server program that actually handles the necessary tasks (for example, responding to incoming connections). The spawned server programs under ```start_server``` call accept(2) and handle the requests.

To gracefully restart the server program, send SIGHUP to the superdaemon. The superdaemon spawns a new server program, and if (and only if) it starts up successfully, sends SIGTERM to the old server program.

By using ```start_server``` it is much easier to write a hot-deployable server. Following are the only requirements a server program to be run under ```start_server``` should conform to:

- receive file descriptors to listen to through an environment variable - perform a graceful shutdown when receiving SIGTERM

Many PSGI servers support this. If you want your Go program to support it, import the root of this module (`github.com/lestrrat-go/server-starter/v2`, see the [package docs](https://github.com/lestrrat-go/server-starter/tree/develop/v2)) for the worker-side implementation, which also fills the ```net.Listener``` interface.

## INSTALLATION

The v2 module is not tagged yet. Install the current v2 revision explicitly:

```
go install github.com/lestrrat-go/server-starter/v2/cmd/start_server@v2.0.0-20260828032310-12627c1634fe
```

Replace the pinned version with `@latest` after v2 is tagged.
