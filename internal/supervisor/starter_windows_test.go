package supervisor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveCommandAgainstDirPreservesExplicitWindowsPaths(t *testing.T) {
	for _, command := range []string{`\worker.exe`, `C:bin\worker.exe`} {
		t.Run(command, func(t *testing.T) {
			resolved, err := resolveCommandAgainstDir(command, `D:\svc`)
			require.NoError(t, err)
			require.Equal(t, command, resolved)
		})
	}
}
