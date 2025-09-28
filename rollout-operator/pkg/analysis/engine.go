package analysis

import "context"

type Spec struct { /* 占位：间隔/阈值/metrics 等，先不用 */
}
type Result struct {
	Passed bool
	Reason string
}
type Engine interface {
	Evaluate(ctx context.Context, s Spec, labels map[string]string) (Result, error)
}
