//go:build !windows

package clipboard

func readFiles() (FileList, error) {
	return FileList{}, nil
}

func writeFiles(paths []string, dropEffect uint32) error {
	return nil
}

func currentSequence() uint32 {
	return 0
}

func readPerformedDropEffect() uint32 {
	return 0
}
