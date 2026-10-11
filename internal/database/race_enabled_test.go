//go:build race

package database

// raceDetectorEnabled reports whether the binary was built with -race.
// Performance acceptance measurements are meaningless under race
// instrumentation, which adds roughly an order of magnitude of overhead, so the
// NFR-U1-PERF assertions skip in that configuration. Race verification is run
// separately with -short.
const raceDetectorEnabled = true
