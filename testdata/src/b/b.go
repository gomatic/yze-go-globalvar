package b

// extra is rebound AND written through outside tests, both of which are
// normally flagged, but the -allow=extra flag permits the name, so neither must
// be flagged here. The allow-list exempts a name from the watch set itself, so
// it covers every kind of write and not only the rebinding it was written for.
var extra = map[string]int{}

func mutate() {
	extra = map[string]int{"a": 1}
	extra["a"] = 2
	delete(extra, "a")
	clear(extra)
}
