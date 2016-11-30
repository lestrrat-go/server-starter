package starter

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func findInOptionList(t *testing.T, opts *options, name string, val interface{}) error {
	for _, o := range makeOptionList(opts) {
		switch o.Name() {
		case name:
			if !assert.Equal(t, val, o.Value(), "option value matches") {
				return errors.New("option value does not match")
			}
			return nil
		}
	}
	t.Errorf("failed to find option")
	return errors.New("failed to find option")
}

func TestCLIArgs(t *testing.T) {
	c := NewCLI()

	t.Run("no parameters", func(t *testing.T) {
		opts, err := c.ParseArgs("ls")
		if !assert.NoError(t, err, "cli.ParseArgs should succeed") {
			t.Logf("%s", err)
			return
		}

		expected := options{
			Command:  "ls",
			Interval: 1,
		}

		if !assert.Equal(t, &expected, opts) {
			return
		}
	})

	t.Run("--interval=0", func(t *testing.T) {
		opts, err := c.ParseArgs("ls", "--interval=0")
		if !assert.NoError(t, err, "cli.ParseArgs should succeed") {
			return
		}

		expected := options{
			Command:  "ls",
			Interval: 0,
		}

		if !assert.Equal(t, &expected, opts) {
			return
		}
		if err := findInOptionList(t, opts, "interval", time.Duration(0)); err != nil {
			return
		}
	})

	t.Run("--dir=foo", func(t *testing.T) {
		opts, err := c.ParseArgs("ls", "--dir=foo")
		if !assert.NoError(t, err, "cli.ParseArgs should succeed") {
			return
		}

		expected := options{
			Command:  "ls",
			Interval: 1,
			Dir:      "foo",
		}

		if !assert.Equal(t, &expected, opts) {
			return
		}
		if err := findInOptionList(t, opts, "dir", "foo"); err != nil {
			return
		}
	})

	for _, val := range []bool{true, false} {
		arg := fmt.Sprintf("--enable-auto-restart=%t", val)
		t.Run(arg, func(t *testing.T) {
			opts, err := c.ParseArgs("ls", arg)
			if !assert.NoError(t, err, "cli.ParseArgs should succeed") {
				return
			}

			expected := options{
				Command:           "ls",
				Interval:          1,
				EnableAutoRestart: boolOpt{ Valid: true, Value: val },
			}

			if !assert.Equal(t, &expected, opts) {
				return
			}
			if err := findInOptionList(t, opts, "enable_auto_restart", val); err != nil {
				return
			}
		})
	}

	for name, sig := range niceNameToSigs {
		hupArg := fmt.Sprintf("--signal-on-hup=%s", name)
		t.Run(hupArg, func(t *testing.T) {
			opts, err := c.ParseArgs("ls", hupArg)
			if !assert.NoError(t, err, "cli.ParseArgs should succeed") {
				return
			}

			expected := options{
				Command:     "ls",
				Interval:    1,
				SignalOnHUP: name,
			}

			if !assert.Equal(t, &expected, opts) {
				return
			}

			if err := findInOptionList(t, opts, "signal_on_hup", sig); err != nil {
				return
			}
		})
		termArg := fmt.Sprintf("--signal-on-term=%s", name)
		t.Run(termArg, func(t *testing.T) {
			opts, err := c.ParseArgs("ls", termArg)
			if !assert.NoError(t, err, "cli.ParseArgs should succeed") {
				return
			}

			expected := options{
				Command:      "ls",
				Interval:     1,
				SignalOnTERM: name,
			}

			if !assert.Equal(t, &expected, opts) {
				return
			}

			if err := findInOptionList(t, opts, "signal_on_term", sig); err != nil {
				return
			}
		})
	}
}
