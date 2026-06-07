package clipboard

type FileList struct {
	Sequence   uint32
	Paths      []string
	DropEffect uint32
}

type PreparedFileList struct {
	Paths          []string
	AfterMovePaste func()
}

const (
	DropEffectCopy uint32 = 1
	DropEffectMove uint32 = 2
)
