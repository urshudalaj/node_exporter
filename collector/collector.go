// Copyright 2015 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package collector provides the framework for implementing node metric collectors.
package collector

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
)

// Namespace defines the common namespace for all node_exporter metrics.
const Namespace = "node"

var (
	scrapeDurationDesc = prometheus.NewDesc(
		prometheus.BuildFQName(Namespace, "scrape", "collector_duration_seconds"),
		"node_exporter: Duration of a collector scrape.",
		[]string{"collector"},
		nil,
	)
	scrapeSuccessDesc = prometheus.NewDesc(
		prometheus.BuildFQName(Namespace, "scrape", "collector_success"),
		"node_exporter: Whether a collector succeeded.",
		[]string{"collector"},
		nil,
	)
)

// Collector is the interface a collector must implement.
type Collector interface {
	// Update pushes collected metrics to the provided channel.
	Update(ch chan<- prometheus.Metric) error
}

// ErrNoData indicates the collector found no data to collect, but had no error.
var ErrNoData = errors.New("collector returned no data")

func isNoDataError(err error) bool {
	return err == ErrNoData
}

// collectorFactory is a function that creates a new Collector.
type collectorFactory func(logger log.Logger) (Collector, error)

var (
	factoriesMu sync.RWMutex
	factories   = make(map[string]collectorFactory)
)

// registerCollector registers a collector factory by name.
func registerCollector(name string, factory collectorFactory) {
	factoriesMu.Lock()
	defer factoriesMu.Unlock()
	factories[name] = factory
}

// NodeCollector implements the prometheus.Collector interface.
type NodeCollector struct {
	collectors map[string]Collector
	logger     log.Logger
}

// NewNodeCollector creates a new NodeCollector with the given collectors enabled.
func NewNodeCollector(logger log.Logger, filters ...string) (*NodeCollector, error) {
	factoriesMu.RLock()
	defer factoriesMu.RUnlock()

	f := make(map[string]bool)
	for _, filter := range filters {
		_, exist := factories[filter]
		if !exist {
			return nil, fmt.Errorf("missing collector: %s", filter)
		}
		f[filter] = true
	}

	collectors := make(map[string]Collector)
	for name, factory := range factories {
		if len(filters) > 0 && !f[name] {
			continue
		}
		c, err := factory(log.With(logger, "collector", name))
		if err != nil {
			// Log the failed collector but continue initializing others rather than
			// aborting the entire NodeCollector setup. This makes startup more resilient
			// on systems where some collectors may not be available.
			level.Warn(logger).Log("msg", "couldn't create collector, skipping", "collector", name, "err", err)
			continue
		}
		collectors[name] = c
	}

	return &NodeCollector{collectors: collectors, logger: logger}, nil
}

// Describe implements the prometheus.Collector interface.
func (n NodeCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- scrapeDurationDesc
	ch <- scrapeSuccessDesc
}

// Collect implements the prometheus.Collector interface.
func (n NodeCollector) Collect(ch chan<- prometheus.Metric) {
	wg := sync.WaitGroup{}
	wg.Add(len(n.collectors))
	for name, c := range n.collectors {
		go func(name string, c Collector) {
			defer wg.Done()
			execute(name, c, ch, n.logger)
		}(name, c)
	}
	wg.Wait()
}

func execute(name string, c Collector, ch chan<- prometheus.Metric, logger log.Logger) {
	begin := time.Now()
	err := c.Update(ch)
	duration := time.Since(begin)
	var success float64

	if err != nil {
		if isNoDataError(err) {
			level.Debug(logger).Log("msg", "collector returned no data", "name", name, "duration_seconds", duration.Seconds(), "err", err)
		} else {
			level.Error(logger).Log("msg", "collector failed", "name", name, "duration_seconds", duration.Seconds(), "err", err)
		}
		success = 0
	} else {
		level.Debug(logger).Log("msg", "collector succeeded", "name", name, "duration_seconds", duration.Seconds())
		success = 1
	}
	ch <- prometheus.MustNewConstMetric(scrapeDurationDesc, prometheus.GaugeValue, duration.Seconds(), name)
	ch <- prometheus.MustNewConstMetric(scrapeSuccessDesc, prometheus.GaugeValue, success, name)
}
