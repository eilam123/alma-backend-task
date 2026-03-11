package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	SpansProcessedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "spans_processed_total",
		Help: "Total number of spans processed.",
	})

	SpansErrorsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "spans_errors_total",
		Help: "Total number of span processing errors.",
	})

	PIIDetectionsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "pii_detections_total",
		Help: "Total number of PII detections by type.",
	}, []string{"type"})

	DBOperationsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "db_operations_total",
		Help: "Total number of database operations by table and operation.",
	}, []string{"table", "operation"})

	SpanProcessingDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "span_processing_duration_seconds",
		Help:    "Duration of span processing in seconds.",
		Buckets: prometheus.DefBuckets,
	})

	APIQueryDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "api_query_duration_seconds",
		Help:    "Duration of API queries in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"endpoint"})

	AppItemsTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "app_items_total",
		Help: "Total number of app items by type.",
	}, []string{"type"})

	ComponentsTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "components_total",
		Help: "Total number of components by type.",
	}, []string{"type"})

	ConnectionsTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "connections_total",
		Help: "Total number of connections.",
	})

	CacheHitsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "api_cache_hits_total",
		Help: "Total number of API cache hits by endpoint.",
	}, []string{"endpoint"})

	CacheMissesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "api_cache_misses_total",
		Help: "Total number of API cache misses by endpoint.",
	}, []string{"endpoint"})

	CacheInvalidationsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "api_cache_invalidations_total",
		Help: "Total number of API cache invalidations.",
	})
)

// Register registers all metrics with the default Prometheus registry.
func Register() {
	prometheus.MustRegister(
		SpansProcessedTotal,
		SpansErrorsTotal,
		PIIDetectionsTotal,
		DBOperationsTotal,
		SpanProcessingDuration,
		APIQueryDuration,
		AppItemsTotal,
		ComponentsTotal,
		ConnectionsTotal,
		CacheHitsTotal,
		CacheMissesTotal,
		CacheInvalidationsTotal,
	)
}
