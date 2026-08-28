package examples_test

import (
	"fmt"
	"os"

	starter "github.com/lestrrat-go/server-starter/v2"
)

// Example_starter_generation shows why Generation returns two values. The
// supervisor sets generation 0 on its own process before it spawns any
// worker, so 0 is a legal, present value — a caller cannot treat a zero
// return as "absent". The bool return is what tells the two cases apart.
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

	// Present: the supervisor sets this on every worker spawn, including
	// generation 0 on its own process before the first worker.
	if err := os.Setenv(starter.GenerationEnvName, "0"); err != nil {
		fmt.Printf("failed to set generation env: %s\n", err)
		return
	}
	generation, ok = starter.Generation()
	fmt.Printf("present: generation=%d ok=%t\n", generation, ok)

	// Output:
	// absent: generation=0 ok=false
	// present: generation=0 ok=true
}
