package application

import "context"

// ReadinessChecker reports whether a dependency is ready for traffic.
type ReadinessChecker interface {
	Check(ctx context.Context) error
}
