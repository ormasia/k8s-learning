package traffic

import (
	"context"
	"fmt"
)

type NginxProvider struct{}

func (p *NginxProvider) SetWeight(ctx context.Context, host, stable, canary string, w int32) error {
	fmt.Printf("[traffic] host=%s weight=%d\n", host, w)
	return nil
}
func (p *NginxProvider) Promote(ctx context.Context, host, stable, canary string) error {
	fmt.Printf("[traffic] promote host=%s\n", host)
	return nil
}
func (p *NginxProvider) Reset(ctx context.Context, host, stable, canary string) error {
	fmt.Printf("[traffic] reset host=%s\n", host)
	return nil
}
