package supervisor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCommandForValidationPreservesExplicitWindowsPaths(t *testing.T) {
	for _, command := range []string{`\worker.exe`, `C:bin\worker.exe`} {
		t.Run(command, func(t *testing.T) {
			require.Equal(t, command, commandForValidation(command, `D:\svc`))
		})
	}
}
