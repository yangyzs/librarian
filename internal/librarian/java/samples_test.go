// Copyright 2024 Google LLC
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
	"testing"
)

func TestDecamelize(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"requesterPays", "Requester Pays"},
		{"ACLBatman", "ACL Batman"},
		{"NativeImageLoggingSample", "Native Image Logging Sample"},
		{"simpleTest", "Simple Test"},
		{"", ""},
	}

	for _, tc := range tests {
		actual := decamelize(tc.input)
		if actual != tc.expected {
			t.Errorf("decamelize(%q) = %q; expected %q", tc.input, actual, tc.expected)
		}
	}
}

func TestExtractSamples_MissingDir(t *testing.T) {
	tempDir := t.TempDir()
	samples, err := ExtractSamples(tempDir)
	if err != nil {
		t.Fatalf("ExtractSamples returned error for missing dir: %v", err)
	}
	if samples != nil {
		t.Errorf("Expected nil samples for missing dir, got %v", samples)
	}
}

func TestExtractSamples_Success(t *testing.T) {
	tempDir := t.TempDir()
	samplesDir := filepath.Join(tempDir, "samples", "src", "main", "java")
	if err := os.MkdirAll(samplesDir, 0755); err != nil {
		t.Fatal(err)
	}

	file1 := filepath.Join(samplesDir, "RequesterPays.java")
	content1 := `// sample-metadata:
//   title: Custom Title Override
//   description: A custom demo sample
public class RequesterPays {}`
	if err := os.WriteFile(file1, []byte(content1), 0644); err != nil {
		t.Fatal(err)
	}

	file2 := filepath.Join(samplesDir, "demoSample.java")
	content2 := `public class demoSample {}`
	if err := os.WriteFile(file2, []byte(content2), 0644); err != nil {
		t.Fatal(err)
	}

	samples, err := ExtractSamples(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(samples) != 2 {
		t.Fatalf("Expected 2 samples, got %d", len(samples))
	}

	// First should be RequesterPays.java with custom YAML override (uppercase R comes before lowercase d)
	s0 := samples[0]
	if s0["Title"] != "Custom Title Override" || s0["title"] != "Custom Title Override" {
		t.Errorf("Sample 0 title = %v; expected 'Custom Title Override'", s0["Title"])
	}
	if s0["File"] != "samples/src/main/java/RequesterPays.java" {
		t.Errorf("Sample 0 file = %v", s0["File"])
	}

	// Second should be demoSample.java
	s1 := samples[1]
	if s1["Title"] != "Demo Sample" || s1["title"] != "Demo Sample" {
		t.Errorf("Sample 1 title = %v; expected 'Demo Sample'", s1["Title"])
	}
	if s1["File"] != "samples/src/main/java/demoSample.java" {
		t.Errorf("Sample 1 file = %v", s1["File"])
	}
}

func TestExtractSnippets(t *testing.T) {
	tempDir := t.TempDir()
	samplesDir := filepath.Join(tempDir, "samples")
	if err := os.MkdirAll(samplesDir, 0755); err != nil {
		t.Fatal(err)
	}

	pomPath := filepath.Join(samplesDir, "pom.xml")
	pomContent := `<project>
  <!-- [START dependency_snippet] -->
  <dependency>
    <groupId>com.google.cloud</groupId>
  </dependency>
  <!-- [END dependency_snippet] -->
</project>`
	if err := os.WriteFile(pomPath, []byte(pomContent), 0644); err != nil {
		t.Fatal(err)
	}

	javaPath := filepath.Join(samplesDir, "Demo.java")
	javaContent := `public class Demo {
  // [START quickstart]
  public void run() {
    // [START_EXCLUDE]
    System.out.println("hidden");
    // [END_EXCLUDE]
    System.out.println("visible");
  }
  // [END quickstart]
}`
	if err := os.WriteFile(javaPath, []byte(javaContent), 0644); err != nil {
		t.Fatal(err)
	}

	snippets, err := ExtractSnippets(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(snippets) != 2 {
		t.Fatalf("Expected 2 snippets, got %d", len(snippets))
	}

	depSnippet := snippets["dependency_snippet"]
	expectedDep := `<dependency>
  <groupId>com.google.cloud</groupId>
</dependency>
`
	if depSnippet != expectedDep {
		t.Errorf("dependency_snippet = %q; expected %q", depSnippet, expectedDep)
	}

	quickSnippet := snippets["quickstart"]
	expectedQuick := `public void run() {
  System.out.println("visible");
}
`
	if quickSnippet != expectedQuick {
		t.Errorf("quickstart = %q; expected %q", quickSnippet, expectedQuick)
	}
}
