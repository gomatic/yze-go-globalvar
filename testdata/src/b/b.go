package b

// extra is reassigned outside tests, which is normally flagged, but the
// -allow=extra flag permits it, so it must NOT be flagged here.
var extra = 1

func mutate() {
	extra = 2
}
