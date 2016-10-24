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

package v2

import (
	"fmt"
	"github.com/fstab/grok_exporter/config/v3"
	"gopkg.in/yaml.v2"
)

func Unmarshal(config []byte) (*v3.Config, error) {
	v2cfg := &Config{}
	err := yaml.Unmarshal(config, v2cfg)
	if err != nil {
		return nil, fmt.Errorf("invalid configuration: %v. make sure to use 'single quotes' around strings with special characters (like match patterns or label templates), and make sure to use '-' only for lists (metrics) but not for maps (labels).", err.Error())
	}
	v3cfg := v2cfg.ToV3()
	err = v3.AddDefaultsAndValidate(v3cfg)
	if err != nil {
		return nil, err
	}
	return v3cfg, nil
}

type Config struct {
	Global  *GlobalConfig  `yaml:",omitempty"`
	Input   *InputConfig   `yaml:",omitempty"`
	Grok    *GrokConfig    `yaml:",omitempty"`
	Metrics *MetricsConfig `yaml:",omitempty"`
	Server  *ServerConfig  `yaml:",omitempty"`
}

type GlobalConfig v3.GlobalConfig

type InputConfig struct {
	Type    string `yaml:",omitempty"`
	Path    string `yaml:",omitempty"`
	Readall bool   `yaml:",omitempty"`
}

type GrokConfig v3.GrokConfig

type MetricsConfig []*MetricConfig

type MetricConfig v3.MetricConfig

type ServerConfig v3.ServerConfig

func (v2cfg *Config) ToV3() *v3.Config {
	return &v3.Config{
		Global:  makeGlobalConfig(),
		Inputs:  convertInputConfig(v2cfg.Input),
		Grok:    (*v3.GrokConfig)(v2cfg.Grok),
		Metrics: convertMetricsConfig(v2cfg.Metrics),
		Server:  (*v3.ServerConfig)(v2cfg.Server),
	}
}

func makeGlobalConfig() *v3.GlobalConfig {
	return &v3.GlobalConfig{
		ConfigVersion: 3,
	}
}

func convertInputConfig(v2inputConfig *InputConfig) *v3.InputsConfig {
	if v2inputConfig == nil {
		return nil
	}
	result := v3.InputsConfig(make([]*v3.InputConfig, 1))
	result[0] = &v3.InputConfig{
		Type:    v2inputConfig.Type,
		Path:    v2inputConfig.Path,
		Readall: v2inputConfig.Readall,
	}
	return &result
}

func convertMetricsConfig(v2metricsConfig *MetricsConfig) *v3.MetricsConfig {
	result := v3.MetricsConfig(make([]*v3.MetricConfig, len(*v2metricsConfig)))
	for i, m := range *v2metricsConfig {
		result[i] = (*v3.MetricConfig)(m)
	}
	return &result
}
