package limiter

// NoOpMetricsRecorder is a placeholder that does nothing.
// It ensures we never have to check 'if r.recorder != nil' in our hot path.
type NoOpMetricsRecorder struct{}

func (n *NoOpMetricsRecorder) Add(name string, value float64, tags map[string]string)     {}
func (n *NoOpMetricsRecorder) Observe(name string, value float64, tags map[string]string) {}
