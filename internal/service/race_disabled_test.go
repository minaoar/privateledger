//go:build !race

package service

// raceDetectorEnabled reports whether the binary was built with -race.
const raceDetectorEnabled = false
