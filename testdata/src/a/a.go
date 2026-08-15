package a

// Mutable is exported: any importer can rebind it, so it is flagged at the declaration.
var Mutable = 1 // want `exported package-level var`

// version is allow-listed (the standard -ldflags stamp) and must NOT be flagged.
var version = "dev"

// Analyzer is allow-listed (the standard analyzer export) and must NOT be flagged.
var Analyzer = 0

// Registration is allow-listed (the standard registration export) and must NOT be flagged.
var Registration = 0

// table is an immutable binding Go cannot express as a const. Nothing rebinds it
// and nothing writes through it outside tests, so it must NOT be flagged — a map
// earns the sanction by being read-only in practice, not by being a map.
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

// rangeAssign rebinds package state through a range clause with Tok ==
// token.ASSIGN — each rebound key/value expression is flagged.
func rangeAssign() {
	for counter = range 3 { // want `reassigned outside tests`
	}
	for counter, length = range []int{1} { // want `reassigned outside tests` `reassigned outside tests`
	}
	for _, length = range []int{1} { // want `reassigned outside tests`
	}
}

// rangeDefine declares new (shadowing) locals via := — it rebinds no package
// state and must NOT be flagged.
func rangeDefine() {
	for counter := range 3 {
		_ = counter
	}
	for range 3 {
	}
}

// parenthesized rebinds package state through parenthesized targets — the
// parens are unwrapped, so each rebinding is flagged.
func parenthesized() {
	(counter) = 5 // want `reassigned outside tests`
	(counter)++   // want `reassigned outside tests`
}

// pointerAlias mutates counter through a pointer alias. The analyzer does no
// escape analysis — mutation reached through an alias rather than through the
// var itself is deliberately out of scope — so this must NOT be flagged. This
// pins the documented contract.
func pointerAlias() {
	p := &counter
	*p = 7
}

// settings, window, matrix and slot hold state a non-test function writes
// THROUGH the var rather than rebinding the var. Package state is immutable
// outside tests, so every such write is flagged at the var it reaches.
type config struct {
	n     int
	inner struct{ n int }
}

var (
	settings config
	window   = make([]int, 4)
	matrix   = [][]int{{1}}
	slot     = new(int)
	store    = map[string]int{"a": 1}
)

// writeThrough writes into package state without ever rebinding a name: a map
// index, a slice index, a struct field, a nested field, a nested index, a
// dereference, a compound assignment and an increment. Each is an assignment
// target rooted at a watched var, so each is flagged.
func writeThrough() {
	store["a"] = 2             // want `mutated outside tests`
	store["a"] += 1            // want `mutated outside tests`
	window[0] = 9              // want `mutated outside tests`
	matrix[0][0] = 9           // want `mutated outside tests`
	settings.n = 7             // want `mutated outside tests`
	settings.n++               // want `mutated outside tests`
	settings.inner.n = 1       // want `mutated outside tests`
	*slot = 3                  // want `mutated outside tests`
	(store)["a"] = 4           // want `mutated outside tests`
	for settings.n = range 3 { // want `mutated outside tests`
	}
}

// builtinWrites removes from, empties and overwrites package state through the
// builtins that write into their first argument. Each is flagged at that
// argument; a later argument is read, not written, so it is not flagged.
func builtinWrites() {
	delete(store, "a")  // want `mutated outside tests`
	clear(store)        // want `mutated outside tests`
	clear(window)       // want `mutated outside tests`
	copy(window, spare) // want `mutated outside tests`
	delete(store, key)  // want `mutated outside tests`
}

// key and spare are watched package vars passed to a builtin as a LATER
// argument, where they are read rather than written. Exactly one finding must
// land on each line of builtinWrites — on the first argument — and none on
// these two, which nothing in this package ever writes to.
var (
	key   = "a"
	spare = []int{1}
)

// readThrough only reads package state. Reading is not mutating, so none of
// these must be flagged — this is the boundary the write cases sit against.
func readThrough() {
	_ = store["a"]
	_ = window[0]
	_ = settings.n
	_ = *slot
	_ = len(store)
	_ = cap(window)
	_ = copy([]int{0}, window)
}

// appendUse passes package state to append, which returns a new slice and is
// not specified to write into the one it was given. It is out of scope for the
// same reason as any other call, and must NOT be flagged; the rebinding of a
// package var from an append result is caught as a rebinding, not here.
func appendUse() {
	_ = append(window, 1)
}

// aliasWrite writes through a local alias of the same underlying map, and
// through a local pointer to a package struct. Both reach package state, and
// both are deliberately out of scope: the analyzer tracks writes rooted at the
// var, not aliasing. This pins the documented limitation beside the in-scope
// siblings in writeThrough that ARE flagged.
func aliasWrite() {
	alias := store
	alias["a"] = 5

	p := &settings
	p.n = 6
}

// callWrite hands package state to a function that writes into it, calls a
// pointer-receiver method on a package var, and sends on a package channel.
// Every one of them really does change package state, and none is an assignment
// target: each is USE of the value, inseparable from the value's ordinary
// purpose. All are out of scope and must NOT be flagged — beside writeThrough's
// in-scope siblings, which are.
func callWrite() {
	fill(store)
	settings.bump()
	pipe <- 1
	<-pipe
}

var pipe = make(chan int, 1)

func fill(m map[string]int) { m["a"] = 1 }

func (cfg *config) bump() { cfg.n++ }

// shadowWrite writes into locals that shadow package vars, and into a local of
// the same shape. A shadowing local resolves to a different object, so none of
// these must be flagged.
func shadowWrite() {
	store := map[string]int{}
	store["a"] = 1
	_ = store

	local := map[string]int{}
	local["a"] = 1
	delete(local, "a")
	clear(local)
}

// rootlessWrite writes into a struct returned by a call. The target is rooted
// at no identifier at all, so there is nothing to resolve and nothing to flag.
func rootlessWrite() {
	provide().n = 1
}

func provide() *config { return &settings }
