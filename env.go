package starter

import (
	"os"
	"strconv"
	"time"
)

func envAsBool(name string) bool {
	b, err := strconv.ParseBool(os.Getenv(name))
	return err == nil && b
}

func envAsInt(name string) int {
	i, _ := strconv.ParseInt(os.Getenv(name), 10, 64)
	return int(i)
}

func envAsDuration(name string) time.Duration {
	return time.Duration(envAsInt(name)) * time.Second
}
