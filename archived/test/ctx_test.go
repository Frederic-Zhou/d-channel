package test

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	go func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				fmt.Println("cancel")
			case <-time.After(2 * time.Second):
				fmt.Println("running")
			}
		}
	}(ctx)

	time.Sleep(3 * time.Second)
	cancel()
	time.Sleep(3 * time.Second)
	cancel()

}
