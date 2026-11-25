package files

import "os"

// CreateCapture creates a capture file below the requested kind while
// preventing path traversal through the filename.
func CreateCapture(kind, filename string) (*os.File, error) {
	base := "./output/" + kind + "/"
	if err := os.MkdirAll(base, 0700); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(base)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.Create(filename)
}
