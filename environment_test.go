package starter_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	starter "github.com/lestrrat-go/server-starter/v2"
)

// unsetEnv unsets name for the duration of the test. t.Setenv records
// whatever value (or absence) name had beforehand and restores it once the
// test finishes, regardless of the value passed here.
func unsetEnv(t *testing.T, name string) {
	t.Helper()

	t.Setenv(name, "")
	require.NoError(t, os.Unsetenv(name))
}

func TestIsUnderStartServer(t *testing.T) {
	t.Run("generation set", func(t *testing.T) {
		t.Setenv(starter.GenerationEnvName, "0")
		require.True(t, starter.IsUnderStartServer())
	})

	t.Run("generation set to non-zero", func(t *testing.T) {
		t.Setenv(starter.GenerationEnvName, "3")
		require.True(t, starter.IsUnderStartServer())
	})

	t.Run("generation unset", func(t *testing.T) {
		unsetEnv(t, starter.GenerationEnvName)
		require.False(t, starter.IsUnderStartServer())
	})

	t.Run("port set but generation unset", func(t *testing.T) {
		t.Setenv(starter.PortEnvName, "")
		unsetEnv(t, starter.GenerationEnvName)
		require.False(t, starter.IsUnderStartServer())
	})
}

func TestGeneration(t *testing.T) {
	t.Run("valid generation", func(t *testing.T) {
		t.Setenv(starter.GenerationEnvName, "5")
		generation, ok := starter.Generation()
		require.True(t, ok)
		require.Equal(t, 5, generation)
	})

	t.Run("generation zero is valid", func(t *testing.T) {
		t.Setenv(starter.GenerationEnvName, "0")
		generation, ok := starter.Generation()
		require.True(t, ok)
		require.Equal(t, 0, generation)
	})

	t.Run("unset", func(t *testing.T) {
		unsetEnv(t, starter.GenerationEnvName)
		generation, ok := starter.Generation()
		require.False(t, ok)
		require.Equal(t, 0, generation)
	})

	t.Run("not an integer", func(t *testing.T) {
		t.Setenv(starter.GenerationEnvName, "not-a-number")
		generation, ok := starter.Generation()
		require.False(t, ok)
		require.Equal(t, 0, generation)
	})
}
