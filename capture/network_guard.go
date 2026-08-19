package capture

// networkGuardCore owns the accounting shared by the live-capture,
// authentication, and author-session network guards. It deliberately has no
// lock: each policy adapter serializes access with its own mutex so policy and
// accounting decisions remain one atomic operation.
type networkGuardCore struct {
	maxRequests      int
	maxResponseBytes int64
	requests         int
	responseBytes    int64
	violation        error
}

func newNetworkGuardCore(maxRequests int, maxResponseBytes int64) networkGuardCore {
	return networkGuardCore{maxRequests: maxRequests, maxResponseBytes: maxResponseBytes}
}

func (g *networkGuardCore) beginRequest(limitViolation error) bool {
	g.requests++
	if g.violation != nil {
		return false
	}
	if g.requests > g.maxRequests {
		g.violate(limitViolation)
		return false
	}
	return true
}

func (g *networkGuardCore) observeResponseContentLength(length int64, sizeViolation error) {
	if length < 0 || length > g.maxResponseBytes {
		g.violate(sizeViolation)
	}
}

func (g *networkGuardCore) observeFinishedResponse(size int64, sizeViolation error) bool {
	if size < 0 || size > g.maxResponseBytes || g.responseBytes > g.maxResponseBytes-size {
		g.violate(sizeViolation)
		return false
	}
	g.responseBytes += size
	return true
}

func (g *networkGuardCore) violate(err error) {
	if g.violation == nil {
		g.violation = err
	}
}

func (g *networkGuardCore) result() error {
	return g.violation
}
