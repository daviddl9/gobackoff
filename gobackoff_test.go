package gobackoff

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestExponentialBackoff_NextBackOff(t *testing.T) {
	t.Run("basic exponential backoff", func(t *testing.T) {
		retries := 4
		b := newDefaultBackoff("TEST", func() (interface{}, error) { return nil, nil })
		for i := 0; i < retries; i++ {
			got := b.getNextBackoffAndIncrementInterval()
			fmt.Println(got)
		}
	})
}

func TestRunWithExponentialBackoff(t *testing.T) {
	ctx := context.Background()
	conf := &Config{
		Enabled:             true,
		RandomizationFactor: 0.5,
		InitialInterval:     1,
		Multiplier:          1.5,
		MaxInterval:         3,
		Retries:             2,
	}

	// Test successful runFunc with no retries
	runFunc := func() (interface{}, error) {
		return "success", nil
	}
	result, err := RunWithExponentialBackoffAndMetrics(ctx, conf, "test1", runFunc)
	assert.Nil(t, err)
	assert.Equal(t, "success", result)

	// Test with nil config
	result, err = RunWithExponentialBackoffAndMetrics(context.Background(), nil, "test", runFunc)
	assert.Nil(t, err)
	assert.Equal(t, "success", result)

	// Test unsuccessful runFunc with retries
	runFunc = func() (interface{}, error) {
		return nil, errors.New("error")
	}
	_, err = RunWithExponentialBackoffAndMetrics(ctx, conf, "test2", runFunc)
	assert.NotNil(t, err)
	assert.Equal(t, "error", err.Error())

	// Test context with deadline
	ctx, _ = context.WithTimeout(ctx, time.Millisecond)
	_, err = RunWithExponentialBackoffAndMetrics(ctx, conf, "test4", runFunc)
	assert.NotNil(t, err)
	assert.Equal(t, "context deadline exceeded", err.Error())

	// Test cancelled context
	ctx, cancelFunc := context.WithCancel(context.Background())
	cancelFunc()
	_, err = RunWithExponentialBackoffAndMetrics(ctx, conf, "test3", runFunc)
	assert.NotNil(t, err)
	assert.Equal(t, "context canceled", err.Error())
}
