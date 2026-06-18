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

// Package postprocessing provides tools for postprocessing generated code.
package postprocessing

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/googleapis/librarian/internal/yaml"
)

var errEmptyNewName = errors.New("new method name is required and cannot be empty")

// Config represents the postprocess.yaml configuration.
type Config struct {
	Replace          []ReplaceConfig      `yaml:"replace,omitempty"`
	ReplaceRegex     []ReplaceRegexConfig `yaml:"replace_regex,omitempty"`
	CopyFile         []CopyConfig         `yaml:"copy_file,omitempty"`
	RemoveFile       []string             `yaml:"remove_file,omitempty"`
	MethodOperations []MethodOperation    `yaml:"method_operations,omitempty"`
}

// MethodOperation represents a method-level operation like delete, duplicate, or deprecate.
type MethodOperation struct {
	Path               string `yaml:"path"`
	Action             string `yaml:"action"`
	FuncName           string `yaml:"func_name"`
	NewName            string `yaml:"new_name,omitempty"`            // Used for duplicate
	DeprecationMessage string `yaml:"deprecation_message,omitempty"` // Used for deprecate
}

// ReplaceConfig represents a replacement rule.
type ReplaceConfig struct {
	Path        string `yaml:"path"`
	Original    string `yaml:"original"`
	Replacement string `yaml:"replacement"`
}

// ReplaceRegexConfig represents a regex replacement rule.
type ReplaceRegexConfig struct {
	Path        string `yaml:"path"`
	Pattern     string `yaml:"pattern"`
	Replacement string `yaml:"replacement"`
}

// CopyConfig represents a file copy rule.
type CopyConfig struct {
	Src string `yaml:"src"`
	Dst string `yaml:"dst"`
}

// Validate validates the postprocess configuration.
func (c *Config) Validate() error {
	for _, r := range c.Replace {
		if strings.TrimSpace(r.Original) == "" {
			return fmt.Errorf("replace rule original text to replace cannot be empty")
		}
	}
	for _, r := range c.ReplaceRegex {
		if strings.TrimSpace(r.Pattern) == "" {
			return fmt.Errorf("replace_regex rule pattern cannot be empty")
		}
	}
	for _, mo := range c.MethodOperations {
		switch mo.Action {
		case "delete":
			if !strings.Contains(mo.FuncName, "(") || !strings.Contains(mo.FuncName, ")") {
				return fmt.Errorf("%w: %q (must contain parameter list in parentheses)", errInvalidSignature, mo.FuncName)
			}
		case "duplicate":
			if !strings.Contains(mo.FuncName, "(") || !strings.Contains(mo.FuncName, ")") {
				return fmt.Errorf("%w: %q (must contain parameter list in parentheses)", errInvalidSignature, mo.FuncName)
			}
			if strings.TrimSpace(mo.NewName) == "" {
				return fmt.Errorf("%w for duplicate method signature %q", errEmptyNewName, mo.FuncName)
			}
		case "deprecate":
			if !strings.Contains(mo.FuncName, "(") || !strings.Contains(mo.FuncName, ")") {
				return fmt.Errorf("%w: %q (must contain parameter list in parentheses)", errInvalidSignature, mo.FuncName)
			}
			if strings.TrimSpace(mo.DeprecationMessage) == "" {
				return fmt.Errorf("%w for deprecating method %q", errEmptyDeprecationMessage, mo.FuncName)
			}
		}
	}
	return nil
}

// ParseConfig parses the postprocess.yaml file.
func ParseConfig(ctx context.Context, path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	configPtr, err := yaml.Unmarshal[Config](data)
	if err != nil {
		return nil, err
	}
	if err := configPtr.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return configPtr, nil
}
