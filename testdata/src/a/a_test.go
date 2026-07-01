package a

// Reassigning a package-level var in a test file is the sanctioned
// dependency-injection seam and must NOT be flagged.
func swapSeam() {
	seam = func() int { return 2 }
	counter = 0
}
