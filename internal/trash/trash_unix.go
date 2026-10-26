//go:build unix

package trash

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func crossDevice(err error) bool {
	return errors.Is(err, syscall.EXDEV)
}

func removeDetached(path string) error {
	cmd := exec.Command("rm", "-rf", "--", path)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return os.RemoveAll(path)
	}
	return cmd.Process.Release()
}
