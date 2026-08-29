//go:build aix || solaris

package supervisor

import "os"

func linkAt(_ *os.File, _ string, _ *os.File, _ string) error {
	return errRenameNoReplaceUnsupported
}
