package starter

import (
	"context"
	"fmt"
	"os/exec"
	"reflect"
	"strconv"
	"time"
)

func ExampleMonitor() {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	ch := make(chan *exec.Cmd, 10)
	done := make(chan *exec.Cmd)
	go monitor(ctx, ch, done)

	for i := 0; i < 10; i++ {
		cmd := exec.Command("sleep", strconv.Itoa(i))
		cmd.Start()
		ch <- cmd
	}

	for {
		select {
		case <-ctx.Done():
			fmt.Printf("timeout reached\n")
			return
		case cmd, ok := <-done:
			if !ok {
				fmt.Println("monitor exited")
				return
			}
			fmt.Printf("notified: %d\n", cmd.ProcessState.Pid())
		}
	}
}

// monitor is a process (i.e. *exec.Cmd) monitor. it asynchronously
// listens for either (a) one of the monitored process exits, or (b)
// we get a request to watch for a new worker.
//
// users of this function can "wait" for the exit of a command
// by checking the done channel
func monitor(ctx context.Context, src chan *exec.Cmd, done chan *exec.Cmd) {
	defer close(done)
	var workers []struct {
		Chan chan error
		Cmd  *exec.Cmd
	}
	for {
		cases := make([]reflect.SelectCase, len(workers)+1)
		for i, worker := range workers {
			cases[i].Chan = reflect.ValueOf(worker.Chan)
			cases[i].Dir  = reflect.SelectRecv
		}
		cases[len(cases)-1].Chan = reflect.ValueOf(src)
		cases[len(cases)-1].Dir = reflect.SelectRecv

		chosen, recv, recvOK := reflect.Select(cases)
		if !recvOK {
			panic("should not get here")
		}

		if chosen == len(workers) {
			// 1 + max worker index, so must be our "new worker chan"
			cmd := recv.Interface().(*exec.Cmd)
			ch := make(chan error)
			go func() {
				ch <- cmd.Wait()
			}()
			workers = append(workers, struct {
				Chan chan error
				Cmd  *exec.Cmd
			}{
				Chan: ch,
				Cmd:  cmd,
			})
			continue
		}

		exited := workers[chosen].Cmd
		// one of the workers must have finished
		// remove the corresponding one
		switch {
		case len(workers) < 2:
			workers = nil
		case chosen == 0:
			workers = workers[1:]
		case chosen == len(workers)-1:
			workers = workers[:chosen]
		default:
			workers = append(workers[:chosen], workers[chosen+1:]...)
		}

		done <- exited
	}
}
