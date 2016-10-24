// Copyright 2016-2017 The grok_exporter Authors
//
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

package main

import (
	"flag"
	"fmt"
	"github.com/fstab/grok_exporter/config"
	"github.com/fstab/grok_exporter/config/v3"
	"github.com/fstab/grok_exporter/exporter"
	"github.com/fstab/grok_exporter/tailer"
	"github.com/prometheus/client_golang/prometheus"
	"net/http"
	"os"
	"time"
)

var (
	printVersion = flag.Bool("version", false, "Print the grok_exporter version.")
	configPath   = flag.String("config", "", "Path to the config file. Try '-config ./example/config.yml' to get started.")
	showConfig   = flag.Bool("showconfig", false, "Print the current configuration to the console. Example: 'grok_exporter -showconfig -config ./exemple/config.yml'")
)

const (
	number_of_lines_matched_label = "matched"
	number_of_lines_ignored_label = "ignored"
)

func main() {
	flag.Parse()
	if *printVersion {
		fmt.Printf("%v\n", exporter.VersionString())
		return
	}
	validateCommandLineOrExit()
	cfg, warn, err := config.LoadConfigFile(*configPath)
	if len(warn) > 0 && !*showConfig {
		// warning is suppressed when '-showconfig' is used
		fmt.Fprintf(os.Stderr, "%v\n", warn)
	}
	exitOnError(err)
	if *showConfig {
		cfg.ClearDefaults()
		fmt.Printf("%v\n", cfg)
		return
	}
	patterns, err := initPatterns(cfg)
	exitOnError(err)
	libonig, err := exporter.InitOnigurumaLib()
	exitOnError(err)
	metrics, err := createMetrics(cfg, patterns, libonig)
	exitOnError(err)
	for _, m := range metrics {
		prometheus.MustRegister(m.Collector())
	}
	nLinesTotal, nMatchesByMetric, procTimeMicrosecondsByMetric, nErrorsByMetric, bufferLoadMetric := initSelfMonitoring(cfg.Global.InputLabelName, cfg.Inputs, metrics)

	tailers := make(map[string]tailer.Tailer)
	for _, inputConfig := range *cfg.Inputs {
		t, err := startTailer(inputConfig, bufferLoadMetric)
		exitOnError(err)
		tailers[inputConfig.InputLabelValue] = t
	}
	allTailers := exporter.RunMultipleFilesTailer(tailers)
	fmt.Print(startMsg(cfg))
	serverErrors := startServer(cfg, "/metrics", prometheus.Handler())

	for {
		select {
		case err := <-serverErrors:
			exitOnError(fmt.Errorf("Server error: %v", err.Error()))
		case err := <-allTailers.Errors():
			exitOnError(fmt.Errorf("Error reading log lines from %v: %v", err.InputLabelValue, err.Error))
		case line := <-allTailers.Lines():
			matched := false
			for _, metric := range metrics {
				start := time.Now()
				match, err := metric.Process(line.InputLabelValue, line.Line)
				if err != nil {
					fmt.Fprintf(os.Stderr, "WARNING: Skipping log line: %v\n", err.Error())
					fmt.Fprintf(os.Stderr, "%v\n", line.Line)
					nErrorsByMetric.WithLabelValues(line.InputLabelValue, metric.Name()).Inc()
				}
				if match {
					nMatchesByMetric.WithLabelValues(line.InputLabelValue, metric.Name()).Inc()
					procTimeMicrosecondsByMetric.WithLabelValues(line.InputLabelValue, metric.Name()).Add(float64(time.Since(start).Nanoseconds() / int64(1000)))
					matched = true
				}
			}
			if matched {
				nLinesTotal.WithLabelValues(line.InputLabelValue, number_of_lines_matched_label).Inc()
			} else {
				nLinesTotal.WithLabelValues(line.InputLabelValue, number_of_lines_ignored_label).Inc()
			}
		}
	}
}

func startMsg(cfg *v3.Config) string {
	host := "localhost"
	if len(cfg.Server.Host) > 0 {
		host = cfg.Server.Host
	} else {
		hostname, err := os.Hostname()
		if err == nil {
			host = hostname
		}
	}
	return fmt.Sprintf("Starting server on %v://%v:%v/metrics\n", cfg.Server.Protocol, host, cfg.Server.Port)
}

func exitOnError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err.Error())
		os.Exit(-1)
	}
}

func validateCommandLineOrExit() {
	if len(*configPath) == 0 {
		if *showConfig {
			fmt.Fprint(os.Stderr, "Usage: grok_exporter -showconfig -config <path>\n")
		} else {
			fmt.Fprint(os.Stderr, "Usage: grok_exporter -config <path>\n")
		}
		os.Exit(-1)
	}
}

func initPatterns(cfg *v3.Config) (*exporter.Patterns, error) {
	patterns := exporter.InitPatterns()
	if len(cfg.Grok.PatternsDir) > 0 {
		err := patterns.AddDir(cfg.Grok.PatternsDir)
		if err != nil {
			return nil, err
		}
	}
	for _, pattern := range cfg.Grok.AdditionalPatterns {
		err := patterns.AddPattern(pattern)
		if err != nil {
			return nil, err
		}
	}
	return patterns, nil
}

func createMetrics(cfg *v3.Config, patterns *exporter.Patterns, libonig *exporter.OnigurumaLib) ([]exporter.Metric, error) {
	result := make([]exporter.Metric, 0, len(*cfg.Metrics))
	for _, m := range *cfg.Metrics {
		regex, err := exporter.Compile(m.Match, patterns, libonig)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize metric %v: %v", m.Name, err.Error())
		}
		err = exporter.VerifyFieldNames(m, regex)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize metric %v: %v", m.Name, err.Error())
		}
		switch m.Type {
		case "counter":
			result = append(result, exporter.NewCounterMetric(cfg.Global.InputLabelName, m, regex))
		case "gauge":
			result = append(result, exporter.NewGaugeMetric(cfg.Global.InputLabelName, m, regex))
		case "histogram":
			result = append(result, exporter.NewHistogramMetric(cfg.Global.InputLabelName, m, regex))
		case "summary":
			result = append(result, exporter.NewSummaryMetric(cfg.Global.InputLabelName, m, regex))
		default:
			return nil, fmt.Errorf("Failed to initialize metrics: Metric type %v is not supported.", m.Type)
		}
	}
	return result, nil
}

func initSelfMonitoring(inputLabelName string, inputsCfg *v3.InputsConfig, metrics []exporter.Metric) (*prometheus.CounterVec, *prometheus.CounterVec, *prometheus.CounterVec, *prometheus.CounterVec, *prometheus.SummaryVec) {
	buildInfo := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "grok_exporter_build_info",
		Help: "A metric with a constant '1' value labeled by version, builddate, branch, revision, goversion, and platform on which grok_exporter was built.",
	}, []string{"version", "builddate", "branch", "revision", "goversion", "platform"})
	nLinesTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "grok_exporter_lines_total",
		Help: "Total number of log lines processed by grok_exporter.",
	}, []string{inputLabelName, "status"})
	nMatchesByMetric := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "grok_exporter_lines_matching_total",
		Help: "Number of lines matched for each metric. Note that one line can be matched by multiple metrics.",
	}, []string{inputLabelName, "metric"})
	procTimeMicrosecondsByMetric := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "grok_exporter_lines_processing_time_microseconds_total",
		Help: "Processing time in microseconds for each metric. Divide by grok_exporter_lines_matching_total to get the averge processing time for one log line.",
	}, []string{inputLabelName, "metric"})
	nErrorsByMetric := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "grok_exporter_line_processing_errors_total",
		Help: "Number of errors for each metric. If this is > 0 there is an error in the configuration file. Check grok_exporter's console output.",
	}, []string{inputLabelName, "metric"})
	bufferLoadMetric := prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Name: "grok_exporter_line_buffer_peak_load",
		Help: "Number of lines that are read from the logfile and waiting to be processed. Peak value per second.",
	}, []string{inputLabelName})

	prometheus.MustRegister(buildInfo)
	prometheus.MustRegister(nLinesTotal)
	prometheus.MustRegister(nMatchesByMetric)
	prometheus.MustRegister(procTimeMicrosecondsByMetric)
	prometheus.MustRegister(nErrorsByMetric)
	prometheus.MustRegister(bufferLoadMetric)

	buildInfo.WithLabelValues(exporter.Version, exporter.BuildDate, exporter.Branch, exporter.Revision, exporter.GoVersion, exporter.Platform).Set(1)
	// Initializing a value with zero makes the label appear. Otherwise the label is not shown until the first value is observed.
	for _, inputCfg := range *inputsCfg {
		nLinesTotal.WithLabelValues(inputCfg.InputLabelValue, number_of_lines_matched_label).Add(0)
		nLinesTotal.WithLabelValues(inputCfg.InputLabelValue, number_of_lines_ignored_label).Add(0)
		bufferLoadMetric.WithLabelValues(inputCfg.InputLabelValue)
		for _, metric := range metrics {
			nMatchesByMetric.WithLabelValues(inputCfg.InputLabelValue, metric.Name()).Add(0)
			procTimeMicrosecondsByMetric.WithLabelValues(inputCfg.InputLabelValue, metric.Name()).Add(0)
			nErrorsByMetric.WithLabelValues(inputCfg.InputLabelValue, metric.Name()).Add(0)
		}
	}
	return nLinesTotal, nMatchesByMetric, procTimeMicrosecondsByMetric, nErrorsByMetric, bufferLoadMetric
}

func startServer(cfg *v3.Config, path string, handler http.Handler) chan error {
	serverErrors := make(chan error)
	go func() {
		switch {
		case cfg.Server.Protocol == "http":
			serverErrors <- exporter.RunHttpServer(cfg.Server.Host, cfg.Server.Port, path, handler)
		case cfg.Server.Protocol == "https":
			if cfg.Server.Cert != "" && cfg.Server.Key != "" {
				serverErrors <- exporter.RunHttpsServer(cfg.Server.Host, cfg.Server.Port, cfg.Server.Cert, cfg.Server.Key, path, handler)
			} else {
				serverErrors <- exporter.RunHttpsServerWithDefaultKeys(cfg.Server.Host, cfg.Server.Port, path, handler)
			}
		default:
			// This cannot happen, because cfg.validate() makes sure that protocol is either http or https.
			serverErrors <- fmt.Errorf("Configuration error: Invalid 'server.protocol': '%v'. Expecting 'http' or 'https'.", cfg.Server.Protocol)
		}
	}()
	return serverErrors
}

func startTailer(cfg *v3.InputConfig, bufferLoadMetric *prometheus.SummaryVec) (tailer.Tailer, error) {
	var tail tailer.Tailer
	switch {
	case cfg.Type == "file":
		tail = tailer.RunFileTailer(cfg.Path, cfg.Readall, nil)
	case cfg.Type == "stdin":
		tail = tailer.RunStdinTailer()
	default:
		return nil, fmt.Errorf("Config error: Input type '%v' unknown.", cfg.Type)
	}
	return exporter.BufferedTailerWithMetrics(tail, cfg.InputLabelValue, bufferLoadMetric), nil
}
