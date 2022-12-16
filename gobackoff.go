package gobackoff

import (
	"context"
	"math/rand"
	"time"

	"github.com/daviddl9/gobackoff/metricsutil"
)

var metricsClient = &MetricsWrapper{}

type MetricsWrapper struct {
	client metricsutil.IMetricsProvider
}

func (m *MetricsWrapper) Increase(metricName string) {
	if m == nil || m.client == nil {
		return
	}
	m.client.Increase(metricName)
}

func InitMetrics(client metricsutil.IMetricsProvider) {
	metricsClient.client = client
}

// Default values for ExponentialBackOff.
const (
	DefaultInitialInterval     = time.Second
	DefaultRandomizationFactor = 0.5
	DefaultMaxInterval         = 3 * time.Second
	DefaultRetries             = 2
	DefaultMultiplier          = 1.5

	failureMetricsName      = ".backoff.failure"
	successMetricsName      = ".backoff.success"
	retrySuccessMetricsName = ".backoff.success_on_retry"
)

type Backoff interface {
	getNextBackoffAndIncrementInterval() time.Duration
}

// ExponentialBackoff is a backoff implementation that increases the backoff period for each retry attempt
// using a randomization function that grows exponentially.
/* getNextBackoffAndIncrementInterval() is calculated using the following formula:

	randomized interval =
	    RetryInterval * (random value in range [1 - RandomizationFactor, 1 + RandomizationFactor])

In other words getNextBackoffAndIncrementInterval() will range between the randomization factor
percentage below and above the retry interval.
For example, given the following parameters:

	InitialInterval = 1s
	MaxInterval = 3s
	RandomizationFactor = 0.5
	Multiplier = 1.5

Here are the distributions of backoff durations for the above default configuration.
Backoff		currentInterval		Actual backoff duration range
1				1s			 	[500ms, 1.5s]
2				1.5s			[750ms, 2.25s]
3				2.25s 			[1.125s, 3s]
4				3s				[1.5s, 3s]

Note: Implementation is not thread-safe.
*/
type ExponentialBackoff[T any] struct {
	InitialInterval     time.Duration
	RandomizationFactor float64
	Multiplier          float64
	MaxInterval         time.Duration
	Retries             int // total retries allowed for backoff. Total tries = Retries + 1

	currentInterval time.Duration
	runFunc         func() (T, error)
	Name            string // name of Backoff function for metrics emission
}

func getDefaultMultiplier(multiplier float64) float64 {
	if multiplier == 0 {
		return DefaultMultiplier
	}
	return multiplier
}

type Config struct {
	Enabled             bool    `json:"enabled"`
	RandomizationFactor float64 `json:"randomization_factor"`
	InitialInterval     float64 `json:"initial_interval"` // time in seconds
	Multiplier          float64 `json:"multiplier"`
	MaxInterval         float64 `json:"max_interval"` // max interval in seconds
	Retries             int     `json:"retries"`      // max number of retries
}

func newDefaultBackoff[T any](name string, runFunc func() (T, error)) *ExponentialBackoff[T] {
	return &ExponentialBackoff[T]{
		InitialInterval:     DefaultInitialInterval,
		RandomizationFactor: DefaultRandomizationFactor,
		Multiplier:          DefaultMultiplier,
		MaxInterval:         DefaultMaxInterval,
		Retries:             DefaultRetries,
		currentInterval:     DefaultInitialInterval,
		runFunc:             runFunc,
		Name:                name,
	}
}

func RunWithExponentialBackoff[T any](ctx context.Context, conf *Config,
	runFunc func() (T, error)) (T, error) {
	if conf == nil {
		return runFunc()
	}
	b := NewExponentialBackoffFromConfig(conf, "", runFunc)
	return b.Run(ctx)
}

func RunWithExponentialBackoffAndMetrics[T any](ctx context.Context, conf *Config, metricsName string,
	runFunc func() (T, error)) (T, error) {
	if conf == nil {
		return runFunc()
	}
	b := NewExponentialBackoffFromConfig(conf, metricsName, runFunc)
	return b.Run(ctx)
}

func NewExponentialBackoffFromConfig[T any](conf *Config, metricsName string,
	runFunc func() (T, error)) *ExponentialBackoff[T] {
	if conf == nil {
		return newDefaultBackoff(metricsName, runFunc)
	}
	return &ExponentialBackoff[T]{
		InitialInterval:     time.Duration(conf.InitialInterval * float64(time.Second)),
		RandomizationFactor: conf.RandomizationFactor,
		Multiplier:          getDefaultMultiplier(conf.Multiplier),
		MaxInterval:         time.Duration(conf.MaxInterval * float64(time.Second)),
		Retries:             conf.Retries,
		currentInterval:     time.Duration(conf.InitialInterval * float64(time.Second)),
		Name:                metricsName,
		runFunc:             runFunc,
	}
}

// NextBackOff calculates the next backoff interval using the formula:
//
//	Randomized interval = RetryInterval * (1 ± RandomizationFactor)
func (b *ExponentialBackoff[T]) getNextBackoffAndIncrementInterval() time.Duration {
	// next backoff needs to be random to prevent thundering herd problem
	next := b.getRandomValueFromInterval(b.RandomizationFactor, b.currentInterval)
	b.incrementCurrentInterval()
	return next
}

// Increments the current interval by multiplying it with the multiplier.
func (b *ExponentialBackoff[T]) incrementCurrentInterval() {
	// Check for overflow, if overflow is detected set the current interval to the max interval.
	b.currentInterval = time.Duration(min(float64(b.currentInterval)*b.Multiplier, float64(b.MaxInterval)))
}

func (b *ExponentialBackoff[T]) Run(ctx context.Context) (T, error) {
	var err error
	res, err := b.runFunc()
	if err == nil {
		b.emitMetrics(b.Name + successMetricsName)
		return res, nil
	}

	totalTries := 1
	for {
		select {
		case <-time.After(b.getNextBackoffAndIncrementInterval()):
			totalTries++
			if totalTries > b.Retries {
				b.emitMetrics(b.Name + failureMetricsName)
				return res, err
			}
			res, err = b.runFunc()
			if err == nil {
				b.emitMetrics(b.Name + retrySuccessMetricsName)
				return res, nil
			}
		case <-ctx.Done():
			return res, ctx.Err()
		}
	}
}

func (b *ExponentialBackoff[T]) isMetricsEnabled() bool {
	return len(b.Name) > 0
}

func (b *ExponentialBackoff[T]) emitMetrics(name string) {
	if !b.isMetricsEnabled() {
		return
	}
	metricsClient.Increase(name)
}

// Returns a random value from the following interval:
//
//	[currentInterval - randomizationFactor * currentInterval, currentInterval + randomizationFactor * currentInterval].
func (b *ExponentialBackoff[T]) getRandomValueFromInterval(randomizationFactor float64,
	currentInterval time.Duration) time.Duration {
	if randomizationFactor == 0 {
		return currentInterval // make sure no randomness is used when randomizationFactor is 0.
	}
	random := rand.Float64() // #nosec
	var delta = randomizationFactor * float64(currentInterval)
	var minInterval = float64(currentInterval) - delta
	var maxInterval = min(float64(currentInterval)+delta, float64(b.MaxInterval))

	// Get a random value from the range [minInterval, maxInterval].
	// The formula used below has a +1 because if the minInterval is 1 and the maxInterval is 3 then
	// we want a 33% chance for selecting either 1, 2 or 3.
	return time.Duration(minInterval + (random * (maxInterval - minInterval + 1)))
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
