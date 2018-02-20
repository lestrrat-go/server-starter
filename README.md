server-starter
=================

Go port of ```start_server``` utility (a.k.a. [Server::Starter](https://metacpan.org/pod/Server::Starter)).

[![Build Status](https://travis-ci.org/lestrrat-go/server-starter.png?branch=master)](https://travis-ci.org/lestrrat-go/server-starter)

[![GoDoc](https://godoc.org/github.com/lestrrat-go/server-starter?status.svg)](https://godoc.org/github.com/lestrrat-go/server-starter)

## DESCRIPTION

*note: this description is almost entirely taken from the original Server::Starter module*

The ```start_server``` utility is a superdaemon for hot-deploying server programs.

It is often a pain to write a server program that supports graceful restarts, with no resource leaks. Server::Starter solves the problem by splitting the task into two: ```start_server``` works as a superdaemon that binds to zero or more TCP ports or unix sockets, and repeatedly spawns the server program that actually handles the necessary tasks (for example, responding to incoming connections). The spawned server programs under ```start_server``` call accept(2) and handle the requests.

To gracefully restart the server program, send SIGHUP to the superdaemon. The superdaemon spawns a new server program, and if (and only if) it starts up successfully, sends SIGTERM to the old server program.

By using ```start_server``` it is much easier to write a hot-deployable server. Following are the only requirements a server program to be run under ```start_server``` should conform to:

- receive file descriptors to listen to through an environment variable - perform a graceful shutdown when receiving SIGTERM

Many PSGI servers support this. If you want your Go program to support it, you can look under the [listener](https://github.com/lestrrat-go/server-starter/tree/master/listener) directory for an implementation that also fills the ```net.Listener``` interface.

## INSTALLATION

```
go get github.com/lestrrat-go/server-starter/cmd/start_server
```
