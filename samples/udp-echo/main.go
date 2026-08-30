package main

import (
	"errors"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"

	starter "github.com/lestrrat-go/server-starter/v2"
)

func main() {
	if !starter.IsUnderStartServer() {
		log.Fatal("UDP echo sample must be started by start_server")
	}

	connections, err := starter.ListenPacketAll()
	if err != nil {
		log.Fatalf("create inherited packet connections: %s", err)
	}

	var wg sync.WaitGroup
	for _, connection := range connections {
		wg.Add(1)
		go func() {
			defer wg.Done()
			echo(connection)
		}()
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	<-signals

	for _, connection := range connections {
		if err := connection.Close(); err != nil {
			log.Printf("close %s: %s", connection.LocalAddr(), err)
		}
	}
	wg.Wait()
}

func echo(connection net.PacketConn) {
	buffer := make([]byte, 64*1024)
	for {
		n, address, err := connection.ReadFrom(buffer)
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				log.Printf("read %s: %s", connection.LocalAddr(), err)
			}
			return
		}
		if _, err := connection.WriteTo(buffer[:n], address); err != nil {
			log.Printf("write %s: %s", connection.LocalAddr(), err)
		}
	}
}
