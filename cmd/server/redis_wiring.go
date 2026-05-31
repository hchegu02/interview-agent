package main

import (
	"context"
	"os"

	"interview-agent/internal/config"
	"interview-agent/internal/httpapi"
)

func buildInterviewEventHub(ctx context.Context, cfg *config.Config) (httpapi.InterviewEventHub, func(), error) {
	if rawURL := os.Getenv("INTERVIEW_REDIS_URL"); rawURL != "" {
		opts, err := httpapi.ParseRedisEventHubOptions(rawURL)
		if err != nil {
			return nil, nil, err
		}
		hub, err := httpapi.NewRedisInterviewEventHub(opts)
		if err != nil {
			return nil, nil, err
		}
		return hub, func() {
			_ = hub.Close()
		}, nil
	}
	hub := httpapi.NewMemoryInterviewEventHub(128)
	return hub, func() {
		_ = hub.Close()
	}, nil
}

func buildRedisSessionCoordinator(ctx context.Context) (*httpapi.RedisSessionCoordinator, error) {
	rawURL := os.Getenv("INTERVIEW_REDIS_URL")
	if rawURL == "" {
		return nil, nil
	}
	opts, err := httpapi.ParseRedisSessionCoordinatorOptions(rawURL)
	if err != nil {
		return nil, err
	}
	return httpapi.NewRedisSessionCoordinator(opts)
}

func eventHubMetricsProvider(events httpapi.InterviewEventHub) func() httpapi.EventHubMetrics {
	switch hub := events.(type) {
	case interface {
		Stats() httpapi.RedisEventHubStats
	}:
		return func() httpapi.EventHubMetrics {
			stats := hub.Stats()
			return httpapi.EventHubMetrics{
				PublishErrors:    stats.PublishErrors,
				DroppedEvents:    stats.DroppedEvents,
				LastPublishError: stats.LastPublishError,
			}
		}
	case interface {
		Stats() httpapi.InterviewEventHubStats
	}:
		return func() httpapi.EventHubMetrics {
			stats := hub.Stats()
			return httpapi.EventHubMetrics{DroppedEvents: stats.DroppedEvents}
		}
	default:
		return nil
	}
}

func hostnameOwnerID() string {
	name, err := os.Hostname()
	if err != nil || name == "" {
		return "local"
	}
	return name
}
