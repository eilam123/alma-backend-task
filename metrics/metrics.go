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
	)
}
