package model

// WANQuality contains the controller's gateway-wide WAN averages for a requested
// health-history window. It is not a per-port or instantaneous measurement.
// Nil fields are unavailable, whereas a pointer to zero is a valid measurement.
type WANQuality struct {
	LatencyMilliseconds *float64
	JitterMilliseconds  *float64
	PacketLossPercent   *float64
	MOS                 *float64
}
