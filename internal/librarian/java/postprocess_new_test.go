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
	t.Chdir(tmpDir)

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

	// Setup postprocess.yaml
	postprocessYaml := `
replace:
  - path: "**/File.java"
    original: "oldFunc"
    replacement: "newFunc"
method_operations:
  - path: "**/File.java"
    action: delete
    func_name: "public void toDelete()"
  - path: "**/File.java"
    action: duplicate
    func_name: "public void newFunc()"
    new_name: "newFuncCopy"
  - path: "**/File.java"
    action: deprecate
    func_name: "public void newFuncCopy()"
    deprecation_message: "Use newFunc instead."
`
	if err := os.WriteFile(filepath.Join(tmpDir, "postprocess.yaml"), []byte(postprocessYaml), 0644); err != nil {
		t.Fatal(err)
	}

	// Setup .repo-metadata.json
	metadata := `{
  "repo": {
    "name_pretty": "My API",
    "distribution_name": "com.google.cloud:google-cloud-myapi",
    "repo": "googleapis/google-cloud-java"
  },
  "library_version": "1.2.3"
}`
	if err := os.WriteFile(filepath.Join(tmpDir, ".repo-metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatal(err)
	}

	oldTemplate := readmeTemplate
	readmeTemplate = `# {{ .Metadata.Repo.NamePretty }}`
	defer func() {
		readmeTemplate = oldTemplate
	}()

	p := libraryPostProcessParams{
		outDir: tmpDir,
		cfg: &config.Config{
			Default: &config.Default{
				Java: &config.JavaModule{
					LibrariesBOMVersion: "1.0.0",
				},
			},
		},
		library: &config.Library{
			Version: "1.2.3",
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
