package env_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/lestrrat/go-server-starter/internal/env"
	"github.com/stretchr/testify/assert"
)

func TestIter(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	src := []string{`FOO=foo`, `BAR=bar`, `BAZ=baz`}
	l := env.NewLoader(src...)
	i := l.Iterator(ctx)
	if !assert.NotNil(t, i, "Iterator is ok") {
		return
	}

	os.Setenv(`QUUX`, `quux`) // This should have no effect
	var list []string
	for i.Next() {
		k, v := i.KV()
		t.Logf("%s=%v", k, v)
		list = append(list, k+"="+v)
	}

	if !assert.Equal(t, src, list) {
		return
	}
}

func TestEnviron(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	src := []string{`FOO=foo`, `BAR=bar`, `BAZ=baz`}
	l := env.NewLoader(src...)
	i := l.Iterator(ctx)
	if !assert.NotNil(t, i, "Iterator is ok") {
		return
	}

	os.Setenv(`QUUX`, `quux`) // This should have no effect
	list := l.Environ(ctx)
	if !assert.Equal(t, src, list) {
		return
	}
}

