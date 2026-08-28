package webapi

import (
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/RCooLeR/omada_exporter/internal/model"
)

func TestEnrichDevicesRunsConcurrentlyWithinLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		devices := make([]model.DevicesInterface, deviceEnrichmentConcurrency*2)
		for i := range devices {
			devices[i] = &model.Switch{}
		}

		started := make(chan struct{}, len(devices))
		release := make(chan struct{})
		finished := make(chan struct{})
		var active atomic.Int32
		var maximum atomic.Int32
		go func() {
			enrichDevices(devices, func(model.DevicesInterface) {
				current := active.Add(1)
				for {
					previous := maximum.Load()
					if current <= previous || maximum.CompareAndSwap(previous, current) {
						break
					}
				}
				started <- struct{}{}
				<-release
				active.Add(-1)
			})
			close(finished)
		}()

		for range deviceEnrichmentConcurrency {
			<-started
		}
		synctest.Wait()
		select {
		case <-started:
			t.Fatalf("more than %d enrichments started concurrently", deviceEnrichmentConcurrency)
		default:
		}
		close(release)

		<-finished
		if got := maximum.Load(); got != deviceEnrichmentConcurrency {
			t.Fatalf("maximum concurrent enrichments = %d, want %d", got, deviceEnrichmentConcurrency)
		}
	})
}
