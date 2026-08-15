// Package c shadows the predeclared delete, clear and copy with package-level
// functions of its own. The builtin write-through check identifies its builtins
// by object, not by spelling, so a call to any of these is an ordinary call —
// out of scope for the same reason every other call is — and must NOT be
// flagged. This is the forgery case for the builtin marker: writing a function
// named clear does not make a call to it a container write the analyzer can see.
package c

var store = map[string]int{"a": 1}

func delete(m map[string]int, key string) { m[key] = 0 }

func clear(m map[string]int) { m["a"] = 0 }

func copy(dst, src []int) {}

// shadowedCalls calls the package's own delete, clear and copy. Every one of
// them really does write into store, and the analyzer is silent on all three
// because a call is not an assignment target — the same limitation aliasWrite
// pins in package a.
func shadowedCalls() {
	delete(store, "a")
	clear(store)
	copy(nil, nil)
}

// realWrite writes into store through an index target, which IS in scope, so it
// is flagged. Without it this package would be silent for the wrong reason and
// would pass against an analyzer that had stopped watching store altogether.
func realWrite() {
	store["a"] = 1 // want `var "store" is mutated`
}
