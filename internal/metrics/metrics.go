package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	NotificationsProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "notifications_processed_total",
			Help: "Total number of notifications processed",
		},
		[]string{"status"}, // status: success, failure
	)

	NotificationProcessingDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "notification_processing_duration_seconds",
			Help:    "Duration of notification processing in seconds",
			Buckets: prometheus.DefBuckets,
		},
	)

	NotificationRetries = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "notification_retries_total",
			Help: "Total number of notification retry attempts",
		},
	)

	ActiveWorkers = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "notification_active_workers",
			Help: "Number of currently active notification workers",
		},
	)

	QueueSize = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "notification_queue_size",
			Help: "Current size of the notification queue",
		},
	)
)
