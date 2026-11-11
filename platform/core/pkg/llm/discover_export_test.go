package llm

import "context"

// DiscoverLocalTargetsForTest probes caller-selected localhost ports without invoking
// the global CLI scanner, keeping discovery tests hermetic across dev environments.
func DiscoverLocalTargetsForTest(ctx context.Context, ports []int) DiscoverLocalResult {
	hints := make([]localPortHint, 0, len(ports))
	for _, p := range ports {
		hints = append(hints, localPortHint{Port: p, Provider: ProviderCustom, Name: "Test Local Engine"})
	}
	return discoverLocalWithTargets(ctx, "127.0.0.1", hints, false, ports)
}
