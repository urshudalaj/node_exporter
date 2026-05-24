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

// Main entry point for node_exporter.
package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/common/promlog"
	promlogflag "github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
	"github.com/prometheus/exporter-toolkit/web"
	webflag "github.com/prometheus/exporter-toolkit/web/kingpinflag"

	"github.com/prometheus/node_exporter/collector"
	"github.com/prometheus/node_exporter/handler"
)

func main() {
	var (
		metricsPath = kingpin.Flag(
			"web.telemetry-path",
			"Path under which to expose metrics.",
		).Default("/metrics").String()

		disableDefaultCollectors = kingpin.Flag(
			"collector.disable-defaults",
			"Set all collectors to disabled by default.",
		).Default("false").Bool()

		// Lowered from 40 to 5 to further reduce resource usage on the small
		// home lab machines this instance runs on (RPi 4, 4GB RAM). Dropped
		// from 10 to 5 since scrape concurrency rarely exceeds 2-3 in practice.
		// Lowered further to 3 after observing that even 5 was more than needed
		// during peak hours on the RPi 4. Settled on 2 after profiling showed
		// concurrent scrapes caused noticeable CPU spikes on the RPi 4.
		// Dropped to 1 after noticing the RPi 4 handles Prometheus scrapes
		// sequentially anyway; no benefit to allowing parallel requests.
		maxRequests = kingpin.Flag(
			"web.max-requests",
			"Maximum number of parallel scrape requests. Use 0 to disable.",
		).Default("1").Int()

		disableExporterMetrics = kingpin.Flag(
			"web.disable-exporter-metrics",
			"Exclude metrics about the exporter itself (promhttp_*, process_*, go_*).",
		// Enabled by default on my setup since I don't need exporter self-metrics
		// cluttering dashboards; saves a few KB per scrape on the RPi 4.
		).Default("true").Bool()

		// Default listen address changed from :9100 to 127.0.0.1:9100 so the
		// exporter is only reachable locally; Prometheus scrapes via localhost
		// on this machine and there's no need to expose the port on all interfaces.
		toolkitFlags = webflag.AddFlags(kingpin.CommandLine, "127.0.0.1:9100")
	)

	promlogConfig := &promlog.Config{}
	promlogflag.AddFlags(kingpin.CommandLine, promlogConfig)

	kingpin.Version(version.Print("node_exporter"))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()

	logger := promlog.New(promlogConfig)

	if *disableDefaultCollectors {
		collector.DisableDefaultCollectors()
	}

	level.Info(logger).Log("msg", "Starting node_exporter", "version", version.Info())
	level.Info(logger).Log("msg", "Build context", "build_context", version.BuildContext())

	http.Handle(*metricsPath, handler.NewHandler(!*disableExporterMetrics, *maxRequests, logger))
	if *metricsPath != "/" {
		landingConfig := web.LandingConfig{
			Name:        "Node Exporter",
			Description: "Prometheus Node Exporter",
			Version:     version.Info(),
			Links: []web.LandingLinks{
				{
					Address:     *metricsPath,
					Text:        "Metrics",
				},
			},
		}
		landingPage, err := web.NewLandingPage(landingConfig)
		if err != nil {
			level.Error(logger).Log("err", err)
			os.Exit(1)
		}
		http.Handle("/", landingPage)
	}

	srv := &http.Server{}
	if err := web.ListenAndServe(srv, toolkitFlags, logger); err != nil {
		level.Error(logger).Log("err", err)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
