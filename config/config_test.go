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

package config

import (
	"strings"
	"testing"
)

const exampleConfig = `
global:
    config_version: 3
inputs:
    - type: file
      path: x/x/x
      readall: true
grok:
    patterns_dir: b/c
metrics:
    - type: counter
      name: test_count_total
      help: Dummy help message.
      match: Some text here, then a %{DATE}.
server:
    protocol: https
    port: 1111
`

func TestVersionDetection(t *testing.T) {
	expectVersion(t, strings.Replace(exampleConfig, "config_version: 3", "config_version: 4", 1), 4, true, false)
	expectVersion(t, strings.Replace(exampleConfig, "config_version: 3", "config_version: 3", 1), 3, false, false)
	expectVersion(t, strings.Replace(exampleConfig, "config_version: 3", "config_version: 2", 1), 2, false, true)
	expectVersion(t, strings.Replace(exampleConfig, "config_version: 3", "config_version: 1", 1), 1, false, true)
	expectVersion(t, strings.Replace(exampleConfig, "config_version: 3", "", 1), 1, false, true)
	expectVersion(t, strings.Replace(exampleConfig, "config_version: 3", "config_version: a", 1), 0, true, false)
}

func expectVersion(t *testing.T, config string, expectedVersion int, errorExpected bool, warningExpected bool) {
	version, warn, err := findVersion(config)
	if errorExpected && err == nil {
		t.Fatalf("didn't get error for config file version.")
	}
	if !errorExpected && err != nil {
		t.Fatalf("unexpected error while getting version info: %v", err.Error())
	}
	if warningExpected && len(warn) == 0 {
		t.Fatalf("didn't get warning for config file version.")
	}
	if !warningExpected && len(warn) > 0 {
		t.Fatalf("unexpected warning: %v", warn)
	}
	if version != expectedVersion {
		t.Fatalf("expected version %v, but found %v", expectedVersion, version)
	}
}
