//go:build !linux

package supervisor

func anchorSocketEntry(string) (func(), error) {
	return func() {}, nil
}
