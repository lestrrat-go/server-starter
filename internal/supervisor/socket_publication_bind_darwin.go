//go:build darwin

package supervisor

import "fmt"

func privateSocketBindPath(privateFD int, name string) string {
	return fmt.Sprintf("/dev/fd/%d/%s", privateFD, name)
}
