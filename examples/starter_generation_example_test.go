package examples_test

import (
	"fmt"
	"os"

	starter "github.com/lestrrat-go/server-starter/v2"
)

// Example_starter_generation shows why Generation returns two values. V2
// workers start at generation 1, but Generation accepts an explicitly
// supplied generation 0 for compatibility. The bool return distinguishes
// that value from an absent variable.
func Example_starter_generation() {
	// Example* functions receive no *testing.T, so t.Setenv is unavailable;
	// save and restore the variable by hand so this example leaves no
	// residue for other tests.
	prior, hadPrior := os.LookupEnv(starter.GenerationEnvName)
	defer func() {
		if hadPrior {
			_ = os.Setenv(starter.GenerationEnvName, prior)
			return
		}
		_ = os.Unsetenv(starter.GenerationEnvName)
	}()

	// Absent: the variable is not set, as when the program was started
	// directly rather than by the supervisor.
	if err := os.Unsetenv(starter.GenerationEnvName); err != nil {
		fmt.Printf("failed to unset generation env: %s\n", err)
		return
	}
	generation, ok := starter.Generation()
	fmt.Printf("absent: generation=%d ok=%t\n", generation, ok)

	// Worker: the v2 supervisor starts workers at generation 1.
	if err := os.Setenv(starter.GenerationEnvName, "1"); err != nil {
		fmt.Printf("failed to set generation env: %s\n", err)
		return
	}
	generation, ok = starter.Generation()
	fmt.Printf("first worker: generation=%d ok=%t\n", generation, ok)

	// Compatibility: Generation accepts an explicitly supplied zero even
	// though the v2 supervisor never emits it for a worker.
	if err := os.Setenv(starter.GenerationEnvName, "0"); err != nil {
		fmt.Printf("failed to set generation env: %s\n", err)
		return
	}
	generation, ok = starter.Generation()
	fmt.Printf("explicit zero: generation=%d ok=%t\n", generation, ok)

	// Output:
	// absent: generation=0 ok=false
	// first worker: generation=1 ok=true
	// explicit zero: generation=0 ok=true
}
