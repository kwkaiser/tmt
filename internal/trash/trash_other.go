//go:build !unix

package trash

import "os"

func crossDevice(error) bool {
	return false
}

func removeDetached(path string) error {
	return os.RemoveAll(path)
}
