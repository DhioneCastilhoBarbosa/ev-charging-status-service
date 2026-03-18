package service

import (
	"context"
	"time"
)

type SchedulerService struct {
	interval time.Duration
}

func NewSchedulerService(interval time.Duration) *SchedulerService {
	return &SchedulerService{interval: interval}
}

func (s *SchedulerService) Run(ctx context.Context, fn func(context.Context)) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fn(ctx)
		}
	}
}

