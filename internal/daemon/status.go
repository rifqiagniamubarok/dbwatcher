package daemon

import (
	"fmt"
	"os"
)

func RemoveFileIfExists(path string) error {
	err := os.Remove(path)
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	return fmt.Errorf("remove %q: %w", path, err)
}
