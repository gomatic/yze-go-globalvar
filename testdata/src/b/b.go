package b

// extra is a package-level var that is normally flagged, but the -allow=extra
// flag permits it, so it must NOT be flagged here.
var extra = 1
