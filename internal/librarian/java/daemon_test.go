// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package java

import (
	"context"
	"testing"
)

func TestGAPICDaemon_StopNil(t *testing.T) {
	var daemon *GAPICDaemon
	if err := daemon.Stop(); err != nil {
		t.Errorf("expected no error when stopping nil daemon, got %v", err)
	}
}

func TestStartDaemonIfConfigured_NilConfig(t *testing.T) {
	ctx := context.Background()
	daemon, err := StartDaemonIfConfigured(ctx, nil)
	if err != nil {
		t.Fatalf("expected no error for nil config, got %v", err)
	}
	if daemon != nil {
		t.Errorf("expected nil daemon for nil config, got %v", daemon)
	}
}
