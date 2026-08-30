//go:build linux

package supervisor

import "fmt"

func privateSocketBindPath(privateFD int, name string) string {
	return fmt.Sprintf("/proc/self/fd/%d/%s", privateFD, name)
}
