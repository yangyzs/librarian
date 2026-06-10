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

// Package postprocessing provides tools for the YAML-based postprocessing workflow.
package postprocessing

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/googleapis/librarian/internal/filesystem"
)

// errTextNotFound is returned when the target text or pattern is not found in the file.
var errTextNotFound = errors.New("text not found")

// CopyFile copies a single file from the src path to the dst path.
// It acts as a wrapper around filesystem.CopyFile to provide a unified
// interface for all postprocessing file operations.
func CopyFile(src, dst string) error {
	return filesystem.CopyFile(src, dst)
}

// Replace replaces all instances of original with replacement in the file at path.
// It supports glob patterns if path contains glob characters.
func Replace(path, original, replacement string) error {
	var files []string
	var err error

	if strings.ContainsAny(path, "*?[]{}") {
		files, err = doublestar.FilepathGlob(path)
		if err != nil {
			return err
		}
	} else {
		files = []string{path}
	}

	for _, f := range files {
		content, err := os.ReadFile(f)
		if err != nil {
			if os.IsNotExist(err) {
				continue // Silently ignore missing files, like synthtool
			}
			return err
		}
		newContent := strings.ReplaceAll(string(content), original, replacement)
		if err := os.WriteFile(f, []byte(newContent), 0644); err != nil {
			return err
		}
	}
	return nil
}

// RemoveFile removes the file at the specified path.
func RemoveFile(path string) error {
	return os.Remove(path)
}

// Replace finds and replaces exact text in a file.
// It returns an error if the target file does not exist or if the text is not found.
func Replace(path, original, replacement string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	oldBytes := []byte(original)
	if !bytes.Contains(content, oldBytes) {
		return fmt.Errorf("%w: %q in file %s", errTextNotFound, original, path)
	}
	newContent := bytes.ReplaceAll(content, oldBytes, []byte(replacement))
	return os.WriteFile(path, newContent, 0644)
}

// ReplaceRegex finds and replaces text in a file using a regular expression.
// It returns an error if the target file does not exist or if the pattern matches no text.
func ReplaceRegex(path, pattern, replacement string) error {
	// Default to multiline mode so ^ and $ match per-line.
	if !strings.HasPrefix(pattern, "(?") {
		pattern = "(?m)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !re.Match(content) {
		return fmt.Errorf("%w: pattern %q in file %s", errTextNotFound, pattern, path)
	}
	newContent := re.ReplaceAll(content, []byte(replacement))
	return os.WriteFile(path, newContent, 0644)
}

// ReplaceRegex replaces all instances of pattern with replacement in the file at path.
// It converts Python-style capture groups like \g<1> to Go-style $1.
// It supports glob patterns if path contains glob characters.
func ReplaceRegex(path, pattern, replacement string) error {
	var files []string
	var err error

	if strings.ContainsAny(path, "*?[]{}") {
		files, err = doublestar.FilepathGlob(path)
		if err != nil {
			return err
		}
	} else {
		files = []string{path}
	}

	// Convert Python-style capture groups \g<1> or \g<name> to Go-style $1 or $name
	rePythonGroup := regexp.MustCompile(`\\g<(\w+)>`)
	replacement = rePythonGroup.ReplaceAllString(replacement, `$$$1`)

	// Convert Python-style numeric capture groups \1, \2 to Go-style $1, $2
	rePythonNumGroup := regexp.MustCompile(`(^|[^\\])\\(\d+)`)
	replacement = rePythonNumGroup.ReplaceAllString(replacement, `${1}$$${2}`)

	if !strings.HasPrefix(pattern, "(?") {
		pattern = "(?m)" + pattern
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}

	for _, f := range files {
		content, err := os.ReadFile(f)
		if err != nil {
			if os.IsNotExist(err) {
				continue // Silently ignore missing files, like synthtool
			}
			return err
		}
		newContent := re.ReplaceAllString(string(content), replacement)
		if err := os.WriteFile(f, []byte(newContent), 0644); err != nil {
			return err
		}
	}
	return nil
}
