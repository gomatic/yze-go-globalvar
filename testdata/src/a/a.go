package a

// Mutable is exported: any importer can rebind it, so it is flagged at the declaration.
var Mutable = 1 // want `exported package-level var`

// version is allow-listed (the standard -ldflags stamp) and must NOT be flagged.
var version = "dev"

// Analyzer is allow-listed (the standard analyzer export) and must NOT be flagged.
var Analyzer = 0

// Registration is allow-listed (the standard registration export) and must NOT be flagged.
var Registration = 0

// table is an immutable binding Go cannot express as a const; never reassigned,
// so it must NOT be flagged.
var table = map[string]int{"a": 1}

// seam is a dependency-injection seam: reassigned only in a_test.go, so the
// declaration and the test reassignment must NOT be flagged.
var seam = func() int { return 1 }

// counter and length are unexported but mutated in production code: each
// rebinding is flagged.
var (
	counter = 0
	length  = 0
	_       = 2
)

// c is a const, not a var, and must NOT be flagged.
const c = 5

// mutate rebinds package state outside tests — every form is flagged.
func mutate() {
	counter++    // want `reassigned outside tests`
	length += 1  // want `reassigned outside tests`
	counter = 2  // want `reassigned outside tests`
	version = "" // allow-listed, not flagged
}

// shadow declares a local counter; rebinding the local must NOT be flagged.
func shadow() {
	counter := 0
	counter = 1
	_ = counter

	local := 3
	_ = local
}
