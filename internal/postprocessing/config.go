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
	"errors"
	"fmt"
	"strings"

	"github.com/googleapis/librarian/internal/config"
)

var errEmptyNewName = errors.New("new method name is required and cannot be empty")

// Validate validates the postprocess configuration.
func Validate(c *config.Postprocess) error {
	if c == nil {
		return nil
	}
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
