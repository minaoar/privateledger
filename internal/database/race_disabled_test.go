//go:build !race

package database

// raceDetectorEnabled reports whether the binary was built with -race.
const raceDetectorEnabled = false
