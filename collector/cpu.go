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

//go:build !nocpu
// +build !nocpu

package collector

import (
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs"
)

const (
	cpuCollectorSubsystem = "cpu"
)

func init() {
	registerCollector("cpu", defaultEnabled, NewCPUCollector)
}

// cpuCollector collects CPU time statistics from /proc/stat.
type cpuCollector struct {
	cpuSecondsTotal *prometheus.Desc
	cpuInfo         *prometheus.Desc
	fs              procfs.FS
	logger          log.Logger
}

// NewCPUCollector returns a new Collector exposing kernel/system statistics.
func NewCPUCollector(logger log.Logger) (Collector, error) {
	fs, err := procfs.NewFS(*procPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open procfs: %w", err)
	}

	return &cpuCollector{
		cpuSecondsTotal: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, cpuCollectorSubsystem, "seconds_total"),
			"Seconds the CPUs spent in each mode.",
			[]string{"cpu", "mode"}, nil,
		),
		cpuInfo: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, cpuCollectorSubsystem, "info"),
			"CPU information from /proc/cpuinfo.",
			[]string{"package", "core", "cpu", "vendor", "family", "model", "model_name", "microcode", "stepping", "cachesize"}, nil,
		),
		fs:     fs,
		logger: logger,
	}, nil
}

// Update implements Collector and exposes cpu time and cpu info stats.
func (c *cpuCollector) Update(ch chan<- prometheus.Metric) error {
	if err := c.updateStat(ch); err != nil {
		return err
	}
	return nil
}

func (c *cpuCollector) updateStat(ch chan<- prometheus.Metric) error {
	stats, err := c.fs.Stat()
	if err != nil {
		return fmt.Errorf("couldn't get stats: %w", err)
	}

	for cpuID, cpuStat := range stats.CPU {
		cpuNum := strconv.Itoa(cpuID)

		// Log a debug message for each CPU being processed, useful for troubleshooting.
		level.Debug(c.logger).Log("msg", "updating CPU stats", "cpu", cpuNum)

		ch <- prometheus.MustNewConstMetric(
			c.cpuSecondsTotal,
			prometheus.CounterValue,
			cpuStat.User,
			cpuNum, "user",
		)
		ch <- prometheus.MustNewConstMetric(
			c.cpuSecondsTotal,
			prometheus.CounterValue,
			cpuStat.Nice,
			cpuNum, "nice",
		)
		ch <- prometheus.MustNewConstMetric(
			c.cpuSecondsTotal,
			prometheus.CounterValue,
			cpuStat.System,
			cpuNum, "system",
		)
		ch <- prometheus.MustNewConstMetric(
			c.cpuSecondsTotal,
			prometheus.CounterValue,
			cpuStat.Idle,
			cpuNum, "idle",
		)
		ch <- prometheus.MustNewConstMetric(
			c.cpuSecondsTotal,
			prometheus.CounterValue,
			cpuStat.Iowait,
			cpuNum, "iowait",
		)
		ch <- prometheus.MustNewConstMetric(
			c.cpuSecondsTotal,
			prometheus.CounterValue,
			cpuStat.IRQ,
			cpuNum, "irq",
		)
		ch <- prometheus.MustNewConstMetric(
			c.cpuSecondsTotal,
			prometheus.CounterValue,
			cpuStat.SoftIRQ,
			cpuNum, "softirq",
		)
		ch <- prometheus.MustNewConstMetric(
			c.cpuSecondsTotal,
			prometheus.CounterValue,
			cpuStat.Steal,
			cpuNum, "steal",
		)
	}
	return nil
}

// Ensure filepath and level imports are used.
var _ = filepath.Join
