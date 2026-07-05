package a

// Export is exported but declared in a _test.go file, which no package can
// import — the export_test.go seam — so it must NOT be flagged.
var Export = seam

// Reassigning a package-level var in a test file is the sanctioned
// dependency-injection seam and must NOT be flagged.
func swapSeam() {
	seam = func() int { return 2 }
	counter = 0
}

// rangeSwap rebinds package state through a range clause in a test file — the
// same sanctioned seam — and must NOT be flagged.
func rangeSwap() {
	for counter = range 3 {
	}
}
