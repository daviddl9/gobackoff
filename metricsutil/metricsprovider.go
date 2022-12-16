package metricsutil

type IMetricsProvider interface {
	Increase(string)
}
