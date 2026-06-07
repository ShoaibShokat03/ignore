//go:build !windows

package clipboard

func readFiles() (FileList, error) {
	return FileList{}, nil
}

func writeFiles(paths []string) error {
	return nil
}

func currentSequence() uint32 {
	return 0
}
