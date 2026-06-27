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
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestDecamelize(t *testing.T) {
	for _, test := range []struct {
		input    string
		expected string
	}{
		{"CamelCase", "Camel Case"},
		{"Word", "Word"},
		{"Camel Case", "Camel Case"},
		{"IamPolicy", "Iam Policy"},
		{"GcsBucket", "Gcs Bucket"},
	} {
		t.Run(test.input, func(t *testing.T) {
			actual := decamelize(test.input)
			if actual != test.expected {
				t.Errorf("decamelize(%q) = %q; expected %q", test.input, actual, test.expected)
			}
		})
	}
}

func TestExtractTitle(t *testing.T) {
	for _, test := range []struct {
		name    string
		content string
		want    string
	}{
		{
			name: "success with standard comment",
			content: `// sample-metadata:
//   title: Standard Title`,
			want: "Standard Title",
		},
		{
			name: "success with indented comment",
			content: `//   sample-metadata:
//     title: Indented Title`,
			want: "Indented Title",
		},
		{
			name: "success with single quotes",
			content: `// sample-metadata:
//   title: 'Single Quotes Title'`,
			want: "Single Quotes Title",
		},
		{
			name: "success with double quotes",
			content: `// sample-metadata:
//   title: "Double Quotes Title"`,
			want: "Double Quotes Title",
		},
		{
			name:    "success with windows carriage returns",
			content: "// sample-metadata:\r\n//   title: Windows Title\r\n",
			want:    "Windows Title",
		},
		{
			name: "no metadata block present",
			content: `// This is a standard java file.
public class Normal {}`,
			want: "",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := extractTitle(test.content)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestExtractTitle_Error(t *testing.T) {
	for _, test := range []struct {
		name    string
		content string
		wantErr error
	}{
		{
			name: "missing title line returns error",
			content: `// sample-metadata:
//   description: No title line immediately following!`,
			wantErr: errMissingTitle,
		},
		{
			name: "empty title value returns error",
			content: `// sample-metadata:
//   title: ""`,
			wantErr: errEmptyTitle,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, gotErr := extractTitle(test.content)
			if !errors.Is(gotErr, test.wantErr) {
				t.Errorf("extractTitle() error = %v, wantErr %v", gotErr, test.wantErr)
			}
		})
	}
}

func TestIsProductionSample(t *testing.T) {
	for _, test := range []struct {
		name string
		path string
		want bool
	}{
		{
			name: "valid production sample",
			path: "samples/src/main/java/com/example/Sample.java",
			want: true,
		},
		{
			name: "valid production sample at root",
			path: "src/main/java/com/example/Sample.java",
			want: true,
		},
		{
			name: "non-java file",
			path: "samples/src/main/java/README.md",
			want: false,
		},
		{
			name: "not in src/main/java",
			path: "samples/src/test/java/com/example/Sample.java",
			want: false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := isProductionSample(test.path)
			if got != test.want {
				t.Errorf("isProductionSample() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestExtractSamples(t *testing.T) {
	for _, test := range []struct {
		name       string
		setupFiles func(t *testing.T, dir string)
		want       []Sample
	}{
		{
			name: "missing samples directory",
			setupFiles: func(t *testing.T, dir string) {
				// Do nothing, tempDir is empty.
			},
			want: nil,
		},
		{
			name: "extract successfully",
			setupFiles: func(t *testing.T, dir string) {
				samplesDir := filepath.Join(dir, "samples", "src", "main", "java")
				if err := os.MkdirAll(samplesDir, 0755); err != nil {
					t.Fatal(err)
				}
				file1 := filepath.Join(samplesDir, "RequesterPays.java")
				content1 := `// sample-metadata:
//   title: Custom Title Override
public class RequesterPays {}`
				if err := os.WriteFile(file1, []byte(content1), 0644); err != nil {
					t.Fatal(err)
				}
				file2 := filepath.Join(samplesDir, "DemoSample.java")
				content2 := `public class DemoSample {}`
				if err := os.WriteFile(file2, []byte(content2), 0644); err != nil {
					t.Fatal(err)
				}
			},
			want: []Sample{
				{
					Title: "Demo Sample",
					File:  "samples/src/main/java/DemoSample.java",
				},
				{
					Title: "Custom Title Override",
					File:  "samples/src/main/java/RequesterPays.java",
				},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			tempDir := t.TempDir()
			test.setupFiles(t, tempDir)

			samples, err := ExtractSamples(tempDir)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(test.want, samples); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestExtractSamples_Error(t *testing.T) {
	for _, test := range []struct {
		name    string
		content string
		wantErr error
	}{
		{
			name: "error on empty title override",
			content: `// sample-metadata:
//   title: ""
public class Invalid {}`,
			wantErr: errEmptyTitle,
		},
		{
			name: "error on capitalized Title",
			content: `// sample-metadata:
//   Title: Capitalized Title
public class Invalid {}`,
			wantErr: errMissingTitle,
		},
		{
			name: "error on missing title line immediately following sample-metadata",
			content: `// sample-metadata:
//   description: missing title line
public class Invalid {}`,
			wantErr: errMissingTitle,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			tempDir := t.TempDir()
			samplesDir := filepath.Join(tempDir, "samples", "src", "main", "java")
			if err := os.MkdirAll(samplesDir, 0755); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(samplesDir, "Sample.java")
			if err := os.WriteFile(file, []byte(test.content), 0644); err != nil {
				t.Fatal(err)
			}
			_, err := ExtractSamples(tempDir)
			if !errors.Is(err, test.wantErr) {
				t.Errorf("ExtractSamples() err = %v, want %v", err, test.wantErr)
			}
		})
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
