package main

import (
	"os"

	"github.com/lestrrat-go/server-starter/v2/internal/cli"
)

func main() {
	os.Exit(cli.Run())
}
