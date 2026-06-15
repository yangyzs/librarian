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

package postprocessing

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseConfig(t *testing.T) {
	yamlContent := `
replace:
  - path: path/to/file.java
    original: "old string"
    replacement: "new string"
replace_regex:
  - path: path/to/file.java
    pattern: "pattern"
    replacement: "replacement"
copy_file:
  - src: path/to/src.java
    dst: path/to/dst.java
remove_file:
  - path/to/file_to_remove.java

method_operations:
  - path: path/to/file.java
    action: delete
    func_name: "public void toDelete()"
  - path: path/to/file.java
    action: duplicate
    func_name: "public void toDuplicate()"
    new_name: "duplicated"
  - path: path/to/file.java
    action: deprecate
    func_name: "public void toDeprecate()"
    deprecation_message: "Use alternative instead."
`
	dir := t.TempDir()
	configPath := filepath.Join(dir, "postprocess.yaml")
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := ParseConfig(context.Background(), configPath)
	if err != nil {
		t.Fatal(err)
	}

	want := &Config{
		Replace: []ReplaceConfig{
			{
				Path:        "path/to/file.java",
				Original:    "old string",
				Replacement: "new string",
			},
		},
		ReplaceRegex: []ReplaceRegexConfig{
			{
				Path:        "path/to/file.java",
				Pattern:     "pattern",
				Replacement: "replacement",
			},
		},
		CopyFile: []CopyConfig{
			{
				Src: "path/to/src.java",
				Dst: "path/to/dst.java",
			},
		},
		RemoveFile: []string{"path/to/file_to_remove.java"},

		MethodOperations: []MethodOperation{
			{
				Path:     "path/to/file.java",
				Action:   "delete",
				FuncName: "public void toDelete()",
			},
			{
				Path:     "path/to/file.java",
				Action:   "duplicate",
				FuncName: "public void toDuplicate()",
				NewName:  "duplicated",
			},
			{
				Path:               "path/to/file.java",
				Action:             "deprecate",
				FuncName:           "public void toDeprecate()",
				DeprecationMessage: "Use alternative instead.",
			},
		},
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("ParseConfig() mismatch (-want +got):\n%s", diff)
	}
}

func TestParseConfig_FileNotFound(t *testing.T) {
	_, err := ParseConfig(context.Background(), "non-existent-file.yaml")
	if err == nil {
		t.Error("ParseConfig() expected error for non-existent file, got nil")
	}
}

func TestParseConfig_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "postprocess.yaml")
	if err := os.WriteFile(configPath, []byte("invalid yaml content"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ParseConfig(context.Background(), configPath)
	if err == nil {
		t.Error("ParseConfig() expected error for invalid YAML, got nil")
	}
}
