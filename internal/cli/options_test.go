package cli

import (
	"os"
	"testing"

	"github.com/lestrrat-go/server-starter/v2/internal/supervisor"
	"github.com/stretchr/testify/require"
)

func TestSignalOptionsRejectInvalidNames(t *testing.T) {
	command, err := os.Executable()
	require.NoError(t, err)

	testCases := []struct {
		name     string
		options  options
		expected string
	}{
		{
			name:     "signal on HUP",
			options:  options{OptCommand: command, OptSignalOnHUP: "TERMM"},
			expected: `signal on HUP: invalid signal name "TERMM"`,
		},
		{
			name:     "signal on TERM",
			options:  options{OptCommand: command, OptSignalOnTERM: "TERMM"},
			expected: `signal on TERM: invalid signal name "TERMM"`,
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			starter, err := supervisor.NewStarter(&test.options)
			require.EqualError(t, err, test.expected)
			require.Nil(t, starter)
		})
	}
}

func TestSignalOptionsAllowDefaults(t *testing.T) {
	command, err := os.Executable()
	require.NoError(t, err)

	starter, err := supervisor.NewStarter(&options{OptCommand: command})
	require.NoError(t, err)
	require.NotNil(t, starter)
}
