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

	t.Run("--auto-restart-interval=5", func(t *testing.T) {
		opts, err := c.ParseArgs("ls", "--auto-restart-interval=5")
		if !assert.NoError(t, err, "cli.ParseArgs should succeed") {
			return
		}

		expected := options{
			Command:             "ls",
			Interval:            1,
			AutoRestartInterval: intOpt{Valid: true, Value: 5},
		}

		if !assert.Equal(t, &expected, opts) {
			return
		}
		if err := findInOptionList(t, opts, "auto_restart_interval", 5*time.Second); err != nil {
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
				EnableAutoRestart: boolOpt{Valid: true, Value: val},
			}

			if !assert.Equal(t, &expected, opts) {
				return
			}
			if err := findInOptionList(t, opts, "enable_auto_restart", val); err != nil {
				return
			}
		})
	}

	t.Run("--envdir=foo", func(t *testing.T) {
		opts, err := c.ParseArgs("ls", "--envdir=foo")
		if !assert.NoError(t, err, "cli.ParseArgs should succeed") {
			return
		}

		expected := options{
			Command:  "ls",
			Interval: 1,
			Envdir:   stringOpt{Valid: true, Value: "foo"},
		}

		if !assert.Equal(t, &expected, opts) {
			return
		}
		if err := findInOptionList(t, opts, "envdir", "foo"); err != nil {
			return
		}
	})

	// 0 is a special case, so we must test
	for i := 0; i < 2; i++ {
		arg := fmt.Sprintf("--interval=%d", i)
		t.Run(arg, func(t *testing.T) {
			opts, err := c.ParseArgs("ls", arg)
			if !assert.NoError(t, err, "cli.ParseArgs should succeed") {
				return
			}

			expected := options{
				Command:  "ls",
				Interval: i,
			}

			if !assert.Equal(t, &expected, opts) {
				return
			}
			if err := findInOptionList(t, opts, "interval", time.Duration(i)*time.Second); err != nil {
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

	for _, i := range []int{5, 10} {
		arg := fmt.Sprintf("--kill-old-delay=%d", i)
		t.Run(arg, func(t *testing.T) {
			opts, err := c.ParseArgs("ls", arg)
			if !assert.NoError(t, err, "cli.ParseArgs should succeed") {
				return
			}

			expected := options{
				Command:      "ls",
				Interval:     1,
				KillOldDelay: intOpt{Valid: true, Value: i},
			}

			if !assert.Equal(t, &expected, opts) {
				return
			}
			if err := findInOptionList(t, opts, "kill_old_delay", time.Duration(i)*time.Second); err != nil {
				return
			}
		})
	}

}
