//go:build race

package main

// raceEnabled skips wall-clock tests: the race detector slows pure-Go SQLite about tenfold.
const raceEnabled = true
