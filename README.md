# gobackoff

A lightweight go library for exponential backoffs with specified retries. Can be helpful for making your systems more fault tolerant, by retrying specific downstream calls.


This implementation was inspired by the following projects: 
- Google's java http client backoff
- This backoff client

Usage:
If you'd like to view metrics on the failure rate, success rate and success after retry rate, you have to initialise your metrics client using the `InitMetrics()` function. The client has to fulfill the metricsprovider interface defined in `metricsprovider.go`.
Without Metrics: 
`gobackoff.RunWithExponentialBackoff(ctx, backoffConfig, runFunc)`
With metrics: 
`RunWithExponentialBackoffAndMetrics[T any](ctx context.Context, conf *Config, metricsName string, runFunc func() (T, error)) (T, error)`