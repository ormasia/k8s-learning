package traffic

import "context"

type Provider interface {
	SetWeight(ctx context.Context, host, stableSvc, canarySvc string, weight int32) error
	Promote(ctx context.Context, host, stableSvc, canarySvc string) error
	Reset(ctx context.Context, host, stableSvc, canarySvc string) error
}
