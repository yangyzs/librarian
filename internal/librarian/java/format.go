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
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/googleapis/librarian/internal/command"
	"github.com/googleapis/librarian/internal/config"
	"golang.org/x/sync/errgroup"
)

const maxFilesPerFormatBatch = 1000

// Format formats Java client libraries using google-java-format in batches.
func Format(ctx context.Context, libraries ...*config.Library) error {
	var allFiles []string
	for _, lib := range libraries {
		files, err := collectJavaFiles(lib.Output)
		if err != nil {
			return fmt.Errorf("failed to find java files for formatting in %q: %w", lib.Name, err)
		}
		allFiles = append(allFiles, files...)
	}
	env, err := getToolsEnv()
	if err != nil {
		return err
	}
	// Batch file paths in chunks of maxFilesPerFormatBatch (1,000 files).
	// Passing 1,000 files per CLI invocation avoids exceeding OS command-line length limits (ARG_MAX)
	// while preventing JVM heap exhaustion on RAM-constrained CI runners.
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(min(runtime.NumCPU(), 2))
	for i := 0; i < len(allFiles); i += maxFilesPerFormatBatch {
		end := min(i+maxFilesPerFormatBatch, len(allFiles))
		chunk := allFiles[i:end]
		g.Go(func() error {
			batchStart := time.Now()
			args := append([]string{"--replace"}, chunk...)
			if err := command.RunWithEnv(gctx, env, "google-java-format", args...); err != nil {
				return fmt.Errorf("failed to format batch [%d:%d]: %w", i, end, err)
			}
			fmt.Printf("[BENCHMARK-CI] Format Batch %d files: %v\n", len(chunk), time.Since(batchStart))
			return nil
		})
	}
	return g.Wait()
}

func collectJavaFiles(root string) ([]string, error) {
	// Attempt to collect modified/untracked Java files using git status to avoid re-formatting unchanged repository files.
	if gitFiles, err := collectGitModifiedJavaFiles(root); err == nil && len(gitFiles) > 0 {
		return gitFiles, nil
	}

	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "target" || (strings.HasPrefix(d.Name(), ".") && d.Name() != ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".java" {
			return nil
		}
		// Exclude generated samples and Spanner-specific sample source directory.
		// Spanner stores its samples in a different location than other libraries.
		// TODO(https://github.com/googleapis/librarian/issues/6095): Remove spanner
		// samples exclusion once we got confirm from the spanner team.
		if strings.Contains(path, filepath.Join("samples", "snippets", "generated")) ||
			strings.Contains(path, filepath.Join("samples", "snippets", "src")) {
			return nil
		}
		files = append(files, path)
		return nil
	})
	return files, err
}

func collectGitModifiedJavaFiles(root string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	topLevel, err := command.Output(ctx, "git", "-C", root, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	topLevel = strings.TrimSpace(topLevel)

	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}

	out, err := command.Output(ctx, "git", "-C", topLevel, "status", "--porcelain", "-u")
	if err != nil {
		return nil, err
	}
	var files []string
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		if len(line) < 4 {
			continue
		}
		// git status --porcelain output format: XY filename or XY -> filename
		relPath := strings.TrimSpace(line[3:])
		if idx := strings.Index(relPath, " -> "); idx != -1 {
			relPath = relPath[idx+4:]
		}
		relPath = strings.Trim(relPath, "\"")
		if filepath.Ext(relPath) != ".java" {
			continue
		}
		if strings.Contains(relPath, "target/") ||
			strings.Contains(relPath, filepath.Join("samples", "snippets", "generated")) ||
			strings.Contains(relPath, filepath.Join("samples", "snippets", "src")) {
			continue
		}
		absPath := filepath.Join(topLevel, relPath)
		if strings.HasPrefix(absPath, absRoot) {
			if _, err := os.Stat(absPath); err == nil {
				files = append(files, absPath)
			}
		}
	}
	if len(files) > 0 {
		fmt.Printf("[BENCHMARK-CI] Formatting %d modified/untracked Java files via git status in %s\n", len(files), root)
	}
	return files, nil
}
