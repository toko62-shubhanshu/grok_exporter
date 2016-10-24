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

package v1

import (
	"fmt"
	"github.com/fstab/grok_exporter/config/v2"
	"github.com/fstab/grok_exporter/config/v3"
	"gopkg.in/yaml.v2"
)

func Unmarshal(config []byte) (*v3.Config, error) {
	v1cfg := &Config{}
	err := yaml.Unmarshal(config, v1cfg)
	if err != nil {
		return nil, fmt.Errorf("invalid configuration: %v", err.Error())
	}
	v3cfg := v1cfg.ToV2().ToV3()
	err = v3.AddDefaultsAndValidate(v3cfg)
	if err != nil {
		return nil, err
	}
	return v3cfg, nil
}

type Config struct {
	// For sections that don't differ between v1 and v2, we reference v2 directly here.
	Input   *InputConfig   `yaml:",omitempty"`
	Grok    *GrokConfig    `yaml:",omitempty"`
	Metrics *MetricsConfig `yaml:",omitempty"`
	Server  *ServerConfig  `yaml:",omitempty"`
}

type InputConfig v2.InputConfig

type GrokConfig v2.GrokConfig

type MetricsConfig []*MetricConfig

type MetricConfig struct {
	Type       string              `yaml:",omitempty"`
	Name       string              `yaml:",omitempty"`
	Help       string              `yaml:",omitempty"`
	Match      string              `yaml:",omitempty"`
	Value      string              `yaml:",omitempty"`
	Cumulative bool                `yaml:",omitempty"`
	Buckets    []float64           `yaml:",flow,omitempty"`
	Quantiles  map[float64]float64 `yaml:",flow,omitempty"`
	Labels     []Label             `yaml:",omitempty"`
}

type Label struct {
	GrokFieldName   string `yaml:"grok_field_name,omitempty"`
	PrometheusLabel string `yaml:"prometheus_label,omitempty"`
}

type ServerConfig v2.ServerConfig

func (v1cfg *Config) ToV2() *v2.Config {
	return &v2.Config{
		Global:  makeGlobalConfig(),
		Input:   (*v2.InputConfig)(v1cfg.Input),
		Grok:    (*v2.GrokConfig)(v1cfg.Grok),
		Metrics: convertMetrics(*v1cfg.Metrics),
		Server:  (*v2.ServerConfig)(v1cfg.Server),
	}
}

func makeGlobalConfig() *v2.GlobalConfig {
	return &v2.GlobalConfig{
		ConfigVersion: 2,
	}
}

func convertMetrics(v1metrics []*MetricConfig) *v2.MetricsConfig {
	if len(v1metrics) == 0 {
		return nil
	}
	v2metrics := make([]*v2.MetricConfig, len(v1metrics))
	for i, v1metric := range v1metrics {
		v2metrics[i] = &v2.MetricConfig{
			Type:       v1metric.Type,
			Name:       v1metric.Name,
			Help:       v1metric.Help,
			Match:      v1metric.Match,
			Value:      makeTemplate(v1metric.Value),
			Cumulative: v1metric.Cumulative,
			Buckets:    v1metric.Buckets,
			Quantiles:  v1metric.Quantiles,
		}
		if len(v1metric.Labels) > 0 {
			v2metrics[i].Labels = make(map[string]string, len(v1metric.Labels))
			for _, v1label := range v1metric.Labels {
				v2metrics[i].Labels[v1label.PrometheusLabel] = makeTemplate(v1label.GrokFieldName)
			}
		}
	}
	result := v2.MetricsConfig(v2metrics)
	return &result
}

func makeTemplate(grokFieldName string) string {
	if len(grokFieldName) > 0 {
		return fmt.Sprintf("{{.%v}}", grokFieldName)
	} else {
		return ""
	}
}
