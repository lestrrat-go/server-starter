package main

import (
	"context"
	"fmt"
	"os"

	"github.com/lestrrat/go-server-starter"
)

func main() {
	cli := starter.NewCLI()
	if err := cli.Run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		os.Exit(1)
	}
}
