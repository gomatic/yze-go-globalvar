package a

// mutable is a plain package-level var and must be flagged.
var mutable = 1 // want `is not permitted`

// version is allow-listed (the standard -ldflags stamp) and must NOT be flagged.
var version = "dev"

// Analyzer is allow-listed (the standard analyzer export) and must NOT be flagged.
var Analyzer = 0

// Registration is allow-listed (the standard registration export) and must NOT be flagged.
var Registration = 0

// A var block: a is flagged; the blank identifier is skipped.
var (
	a = 1 // want `is not permitted`
	_ = 2
)

// c is a const, not a var, and must NOT be flagged.
const c = 5

// f has a function-local var, which is not a package-level var and must NOT be flagged.
func f() {
	local := 3
	_ = local
}
