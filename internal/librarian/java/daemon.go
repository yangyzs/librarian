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
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/googleapis/librarian/internal/config"
)

// GAPICDaemon manages a long-running background JVM process using Nailgun for GAPIC code generation.
type GAPICDaemon struct {
	cmd  *exec.Cmd
	Port int
}

// StartGAPICDaemon starts a background JVM daemon process running com.martiansoftware.nailgun.NGServer.
func StartGAPICDaemon(ctx context.Context, toolsEnv map[string]string, classpath string, port int) (*GAPICDaemon, error) {
	cmd := exec.CommandContext(ctx, "java",
		"-Xms512m",
		"-Xmx5g",
		"-XX:+UseG1GC",
		"--add-exports=jdk.compiler/com.sun.tools.javac.api=ALL-UNNAMED",
		"--add-exports=jdk.compiler/com.sun.tools.javac.file=ALL-UNNAMED",
		"--add-exports=jdk.compiler/com.sun.tools.javac.parser=ALL-UNNAMED",
		"--add-exports=jdk.compiler/com.sun.tools.javac.tree=ALL-UNNAMED",
		"--add-exports=jdk.compiler/com.sun.tools.javac.util=ALL-UNNAMED",
		"--add-opens=jdk.compiler/com.sun.tools.javac.code=ALL-UNNAMED",
		"--add-opens=jdk.compiler/com.sun.tools.javac.comp=ALL-UNNAMED",
		"-cp", classpath,
		"com.martiansoftware.nailgun.NGServer",
		"127.0.0.1",
		fmt.Sprintf("%d", port),
	)
	cmd.Env = os.Environ()
	for k, v := range toolsEnv {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start nailgun daemon: %w", err)
	}

	// Wait for socket port to become ready
	address := fmt.Sprintf("127.0.0.1:%d", port)
	for i := 0; i < 50; i++ {
		conn, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return &GAPICDaemon{cmd: cmd, Port: port}, nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	_ = cmd.Process.Kill()
	return nil, fmt.Errorf("nailgun daemon failed to respond on port %d within 5 seconds", port)
}

// Stop terminates the background JVM daemon process.
func (d *GAPICDaemon) Stop() error {
	os.Unsetenv("NAILGUN_PORT")
	if d != nil && d.cmd != nil && d.cmd.Process != nil {
		return d.cmd.Process.Kill()
	}
	return nil
}

// StartDaemonIfConfigured attempts to start a GAPIC JVM daemon if configured tools are present.
func StartDaemonIfConfigured(ctx context.Context, cfg *config.Config) (*GAPICDaemon, error) {
	if cfg == nil || cfg.Tools == nil {
		return nil, nil
	}
	env, err := getToolsEnv()
	if err != nil {
		return nil, err
	}
	libDir, err := getLibDir()
	if err != nil {
		return nil, err
	}
	nailgunMatches, _ := filepath.Glob(filepath.Join(libDir, "nailgun-server-*.jar"))
	if len(nailgunMatches) == 0 {
		return nil, nil // Safe fallback if nailgun server jar is not installed
	}

	allJars, _ := filepath.Glob(filepath.Join(libDir, "*.jar"))
	if len(allJars) == 0 {
		return nil, nil
	}
	classpath := strings.Join(allJars, ":")

	daemon, err := StartGAPICDaemon(ctx, env, classpath, 2113)
	if err != nil {
		return nil, err
	}
	os.Setenv("NAILGUN_PORT", "2113")
	return daemon, nil
}
