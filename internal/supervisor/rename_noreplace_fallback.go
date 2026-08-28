package supervisor

import (
	"errors"
	"os"
)

var errRenameNoReplaceUnsupported = errors.New("atomic no-replace rename is unsupported on this platform")

func unsupportedRenameNoReplaceAt(_ *os.File, _, _ string) error {
	return errRenameNoReplaceUnsupported
}
