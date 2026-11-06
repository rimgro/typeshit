//go:build race

package effects

// raceEnabled disables wall-clock assertions under the race detector, which
// slows raster code by an order of magnitude.
const raceEnabled = true
