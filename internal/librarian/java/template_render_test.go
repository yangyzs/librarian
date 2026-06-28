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
	"text/template"

	"github.com/google/go-cmp/cmp"
)

func TestRenderREADME(t *testing.T) {
	tmpDir := t.TempDir()

	templateContent := `# Google {{ .Metadata.Repo.NamePretty }} Client for Java
Artifact: {{ .GroupID }}:{{ .ArtifactID }}
Version: {{ .Version }}
BOMVersion: {{ .BOMVersion }}
LibraryVersion: {{ .LibraryVersion }}
{{ if and .Metadata.Partials .Metadata.Partials.About }}
About: {{ .Metadata.Partials.About }}
{{ end }}
`
	mockTmpl := template.Must(template.New("mock").Parse(templateContent))

	metadata := &repoMetadata{
		NamePretty:       "My API",
		DistributionName: "com.google.cloud:google-cloud-myapi",
		Repo:             "googleapis/google-cloud-java",
	}

	// Test case 1: Without partials
	err := renderREADMEWithTemplate(tmpDir, metadata, "1.0.0-BOM", "1.2.3-LIB", nil, mockTmpl)
	if err != nil {
		t.Fatal(err)
	}

	outputPath := filepath.Join(tmpDir, "README.md")
	outputContent, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}

	expected := `# Google My API Client for Java
Artifact: com.google.cloud:google-cloud-myapi
Version: 1.2.3-LIB
BOMVersion: 1.0.0-BOM
LibraryVersion: 1.2.3-LIB
`
	if diff := cmp.Diff(strings.TrimSpace(expected), strings.TrimSpace(string(outputContent))); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}

	// Test case 2: With partials
	partialsPath := filepath.Join(tmpDir, ".readme-partials.yaml")
	partialsContent := `about: "This is a great API."`
	err = os.WriteFile(partialsPath, []byte(partialsContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	err = renderREADMEWithTemplate(tmpDir, metadata, "1.0.0-BOM", "1.2.3-LIB", nil, mockTmpl)
	if err != nil {
		t.Fatal(err)
	}

	outputContent, err = os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}

	expectedWithPartials := `# Google My API Client for Java
Artifact: com.google.cloud:google-cloud-myapi
Version: 1.2.3-LIB
BOMVersion: 1.0.0-BOM
LibraryVersion: 1.2.3-LIB

About: This is a great API.
`
	if diff := cmp.Diff(strings.TrimSpace(expectedWithPartials), strings.TrimSpace(string(outputContent))); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}

	// Test case 3: With README.md in keep list
	keepSet := map[string]bool{"README.md": true}
	customContent := "Custom README content"
	err = os.WriteFile(outputPath, []byte(customContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	err = renderREADMEWithTemplate(tmpDir, metadata, "1.0.0-BOM", "1.2.3-LIB", keepSet, mockTmpl)
	if err != nil {
		t.Fatal(err)
	}

	outputContent, err = os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(customContent, string(outputContent)); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestRealTemplateParses(t *testing.T) {
	if readmeTmplParsed == nil {
		t.Fatal("readmeTmplParsed is nil")
	}
}
