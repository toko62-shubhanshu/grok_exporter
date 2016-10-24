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

package exporter

import (
	"github.com/fstab/grok_exporter/tailer"
)

type MultipleFilesTailer struct {
	tailers map[string]tailer.Tailer
	lines   chan MultipleFilesTailerEvent
	errors  chan MultipleFilesTailerError
	done    chan interface{}
}

type MultipleFilesTailerEvent struct {
	InputLabelValue string
	Line            string
}

type MultipleFilesTailerError struct {
	InputLabelValue string
	Error           error
}

func RunMultipleFilesTailer(tailers map[string]tailer.Tailer) *MultipleFilesTailer {
	multiTail := &MultipleFilesTailer{
		tailers: tailers,
		lines:   make(chan MultipleFilesTailerEvent),
		errors:  make(chan MultipleFilesTailerError),
		done:    make(chan interface{}),
	}
	for inputLabelValue, tail := range multiTail.tailers {
		go func(inputLabelValue string, tail tailer.Tailer) {
			for {
				select {
				case line := <-tail.Lines():
					multiTail.lines <- MultipleFilesTailerEvent{
						Line:            line,
						InputLabelValue: inputLabelValue,
					}
				case err := <-tail.Errors():
					multiTail.errors <- MultipleFilesTailerError{
						Error:           err,
						InputLabelValue: inputLabelValue,
					}
				case <-multiTail.done:
					tail.Close()
					break
				}
			}
		}(inputLabelValue, tail)
	}
	return multiTail
}

func (m *MultipleFilesTailer) Lines() chan MultipleFilesTailerEvent {
	return m.lines
}

func (m *MultipleFilesTailer) Errors() chan MultipleFilesTailerError {
	return m.errors
}

func (m *MultipleFilesTailer) Close() {
	close(m.done)
}
