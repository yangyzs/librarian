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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/googleapis/librarian/internal/config"
)

func TestPostProcessLibraryNew(t *testing.T) {
	tmpDir := t.TempDir()

	// Setup structure directly in outDir
	destDir := filepath.Join(tmpDir, "my-module", "src", "main", "java")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		t.Fatal(err)
	}

	fileContent := `package com.example;
public class File {
	public void oldFunc() {}
	public void toDelete() {
		System.out.println("delete me");
	}
}`
	filePath := filepath.Join(destDir, "File.java")
	if err := os.WriteFile(filePath, []byte(fileContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Write mock template to disk
	tmplDir := filepath.Join(tmpDir, "template")
	if err := os.MkdirAll(tmplDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmplDir, "README.md.go.tmpl"), []byte(`# {{ .Metadata.Repo.NamePretty }}`), 0644); err != nil {
		t.Fatal(err)
	}

	p := libraryPostProcessParams{
		outDir: tmpDir,
		cfg: &config.Config{
			Default: &config.Default{
				Java: &config.JavaDefault{
					LibrariesBOMVersion: "1.0.0",
				},
			},
		},
		library: &config.Library{
			Version: "1.2.3",
			Postprocess: &config.Postprocess{
				Replace: []config.ReplaceConfig{
					{
						Path:        "**/File.java",
						Original:    "oldFunc",
						Replacement: "newFunc",
					},
				},
				MethodOperations: []config.MethodOperation{
					{
						Path:     "**/File.java",
						Action:   "delete",
						FuncName: "public void toDelete()",
					},
					{
						Path:     "**/File.java",
						Action:   "duplicate",
						FuncName: "public void newFunc()",
						NewName:  "newFuncCopy",
					},
					{
						Path:               "**/File.java",
						Action:             "deprecate",
						FuncName:           "public void newFuncCopy()",
						DeprecationMessage: "Use newFunc instead.",
					},
				},
			},
		},
		metadata: &repoMetadata{
			NamePretty:       "My API",
			DistributionName: "com.google.cloud:google-cloud-myapi",
			Repo:             "googleapis/google-cloud-java",
		},
	}

	err := postProcessLibraryNew(t.Context(), p)
	if err != nil {
		t.Fatal(err)
	}

	// Verify File was modified
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	sContent := string(content)
	if !strings.Contains(sContent, "newFunc") {
		t.Errorf("Replacement was not applied. Content: %s", sContent)
	}
	if strings.Contains(sContent, "toDelete") {
		t.Errorf("Delete function was not applied. Content: %s", sContent)
	}
	if !strings.Contains(sContent, "newFuncCopy") {
		t.Errorf("Duplicate method operation was not applied. Content: %s", sContent)
	}
	if !strings.Contains(sContent, "@Deprecated\n\tpublic void newFuncCopy()") {
		t.Errorf("@Deprecated annotation was not applied correctly. Content: %s", sContent)
	}
	if !strings.Contains(sContent, "* @deprecated Use newFunc instead.") {
		t.Errorf("Javadoc deprecation tag was not applied correctly. Content: %s", sContent)
	}

	// Verify README was rendered
	readmePath := filepath.Join(tmpDir, "README.md")
	if _, err := os.Stat(readmePath); err != nil {
		t.Errorf("README.md was not rendered: %v", err)
	}
	readmeContent, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(readmeContent) != "# My API" {
		t.Errorf("README content mismatch. Got: %s, expected: # My API", string(readmeContent))
	}
}
