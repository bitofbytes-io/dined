// Package metricstest reads dined's metrics in tests.
package metricstest

import (
	"testing"

	"github.com/bitofbytes-io/dined/internal/metrics"
)

// Value returns the counter or gauge series name whose labels are exactly
// labels, or 0 when that series has not been recorded. The metrics are
// process-wide, so callers should compare values before and after an action.
func Value(t testing.TB, name string, labels map[string]string) float64 {
	t.Helper()
	families, err := metrics.NewRegistry().Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
	series:
		for _, m := range family.GetMetric() {
			if len(m.GetLabel()) != len(labels) {
				continue
			}
			for _, pair := range m.GetLabel() {
				if want, ok := labels[pair.GetName()]; !ok || want != pair.GetValue() {
					continue series
				}
			}
			if m.GetCounter() != nil {
				return m.GetCounter().GetValue()
			}
			return m.GetGauge().GetValue()
		}
	}
	return 0
}
