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
	"bufio"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"

	"github.com/googleapis/librarian/internal/yaml"
	"github.com/iancoleman/strcase"
)

var (
	//go:embed template/README.md.go.tmpl
	readmeTmpl       string
	readmeTmplParsed = template.Must(template.New("README").Parse(readmeTmpl))

	openSnippetRegex  = regexp.MustCompile(`\[START ([a-zA-Z0-9_-]+)\]`)
	closeSnippetRegex = regexp.MustCompile(`\[END ([a-zA-Z0-9_-]+)\]`)
	openExcludeRegex  = regexp.MustCompile(`\[START_EXCLUDE\]`)
	closeExcludeRegex = regexp.MustCompile(`\[END_EXCLUDE\]`)

	// Matches lowercase/digit followed by uppercase (e.g., "FooBar" -> "Foo Bar").
	camelCaseRegexp = regexp.MustCompile(`([a-z0-9])([A-Z])`)

	// reTitle matches a "sample-metadata:" marker followed immediately by a "title:" line on the next comment line.
	reTitle = regexp.MustCompile(`sample-metadata:\s*\n\s*(?://|#)\s*title:\s*(.*)`)

	// errMissingTitle indicates the "title:" line is missing immediately following "sample-metadata:".
	errMissingTitle = errors.New("missing title line immediately following sample-metadata")

	// errEmptyTitle indicates the extracted title value is empty.
	errEmptyTitle = errors.New("title value cannot be empty")
)

// Sample represents a Java code sample and its metadata for README generation.
type Sample struct {
	Title string
	File  string
}

// ExtractSamples walks the "samples" directory locating all .java source files.
// It extracts title overrides from source file comments using extractTitle.
func ExtractSamples(dir string) ([]Sample, error) {
	samplesDir := filepath.Join(dir, "samples")
	var files []string
	err := filepath.WalkDir(samplesDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if isProductionSample(rel) {
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk samples directory: %w", err)
	}

	var samples []Sample
	for _, file := range files {
		base := strings.TrimSuffix(filepath.Base(file), ".java")
		title := decamelize(base)
		slashPath := filepath.ToSlash(file)

		absPath := filepath.Join(dir, file)
		contentBytes, err := os.ReadFile(absPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read sample file %s: %w", file, err)
		}
		titleOverride, err := extractTitle(string(contentBytes))
		if err != nil {
			return nil, fmt.Errorf("failed to extract title from %s: %w", file, err)
		}
		if titleOverride != "" {
			title = titleOverride
		}
		samples = append(samples, Sample{
			Title: title,
			File:  slashPath,
		})
	}
	return samples, nil
}

// decamelize converts CamelCase string to space-separated string (e.g. "CamelCase" -> "Camel Case").
func decamelize(value string) string {
	return strings.TrimSpace(camelCaseRegexp.ReplaceAllString(value, `$1 $2`))
}

// isProductionSample reports whether the given path represents a production Java source file
// located under a standard "src/main/java" path.
func isProductionSample(path string) bool {
	slashed := filepath.ToSlash(path)
	return strings.HasSuffix(slashed, ".java") &&
		(strings.HasPrefix(slashed, "src/main/java/") || strings.Contains(slashed, "/src/main/java/"))
}

// extractTitle extracts and validates the title override from Java comment blocks.
// It expects a "title:" line to immediately follow the "sample-metadata:" marker.
// Returns an error if the marker is present but the title line is missing, malformed, or empty.
func extractTitle(content string) (string, error) {
	if !strings.Contains(content, "sample-metadata:") {
		return "", nil
	}
	matches := reTitle.FindStringSubmatch(content)
	if len(matches) < 2 {
		return "", errMissingTitle
	}
	// Trim surrounding whitespace, quotes, and carriage returns.
	title := strings.Trim(matches[1], " \t\"'\r\n")
	if title == "" {
		return "", errEmptyTitle
	}
	return title, nil
}

// ExtractSnippets walks the "samples" directory locating *.java and *.xml files.
// It line-scans for [START name] and [END name] tags while supporting [START_EXCLUDE] blocks,
// returning trimmed minimum plain-space indentation blocks.
func ExtractSnippets(dir string) (map[string]string, error) {
	samplesDir := filepath.Join(dir, "samples")
	var files []string

	err := filepath.WalkDir(samplesDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			if d.Name() == "test" {
				return filepath.SkipDir
			}
			if d.Name() == "generated" && filepath.Base(filepath.Dir(path)) == "snippets" {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		ext := filepath.Ext(path)
		if ext == ".java" || ext == ".xml" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		return nil, nil
	}

	sort.Strings(files)
	snippetLines := make(map[string][]string)

	for _, file := range files {
		if err := extractSnippetsFromFile(file, snippetLines); err != nil {
			return nil, err
		}
	}

	if len(snippetLines) == 0 {
		return nil, nil
	}

	result := make(map[string]string)
	for snippet, lines := range snippetLines {
		result[snippet] = trimLeadingWhitespace(lines)
	}
	return result, nil
}

// trimLeadingWhitespace computes the minimum plain-space indentation across non-empty lines,
// trimming that common whitespace while preserving newlines.
func trimLeadingWhitespace(lines []string) string {
	if len(lines) == 0 {
		return ""
	}

	minSpaces := -1
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			spaces := len(line) - len(strings.TrimLeft(line, " "))
			if minSpaces == -1 || spaces < minSpaces {
				minSpaces = spaces
			}
		}
	}
	if minSpaces == -1 {
		minSpaces = 0
	}

	var sb strings.Builder
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			sb.WriteString("\n")
		} else {
			if len(line) >= minSpaces {
				sb.WriteString(line[minSpaces:])
			} else {
				sb.WriteString(strings.TrimLeft(line, " "))
			}
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// extractSnippetsFromFile parses a single file to extract tagged code snippets.
func extractSnippetsFromFile(file string, snippetLines map[string][]string) error {
	f, err := os.Open(file)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", file, err)
	}
	defer f.Close()

	openSnippets := make(map[string]bool)
	excluding := false
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		openMatch := openSnippetRegex.FindStringSubmatch(line)
		closeMatch := closeSnippetRegex.FindStringSubmatch(line)

		if len(openMatch) > 1 && !excluding {
			name := openMatch[1]
			openSnippets[name] = true
			if _, exists := snippetLines[name]; !exists {
				snippetLines[name] = []string{}
			}
		} else if len(closeMatch) > 1 && !excluding {
			delete(openSnippets, closeMatch[1])
		} else if openExcludeRegex.MatchString(line) {
			excluding = true
		} else if closeExcludeRegex.MatchString(line) {
			excluding = false
		} else if !excluding {
			for s := range openSnippets {
				snippetLines[s] = append(snippetLines[s], line)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed scanning file %s: %w", file, err)
	}
	return nil
}

// RenderREADME renders the README.md file using the template and metadata.
// dir is the directory containing where README.md will be written.
func RenderREADME(dir string, metadata *repoMetadata, bomVersion, libraryVersion string, keepSet map[string]bool) error {
	return renderREADMEWithTemplate(dir, metadata, bomVersion, libraryVersion, keepSet, readmeTmplParsed)
}

func renderREADMEWithTemplate(dir string, metadata *repoMetadata, bomVersion, libraryVersion string, keepSet map[string]bool, tmpl *template.Template) error {
	outputPath := filepath.Join(dir, "README.md")
	if keepSet["README.md"] {
		return nil
	}

	partialsPath := filepath.Join(dir, ".readme-partials.yaml")
	if _, err := os.Stat(partialsPath); errors.Is(err, fs.ErrNotExist) {
		partialsPath = filepath.Join(dir, ".readme-partials.yml")
	}

	// Read partials if exist
	var partials map[string]interface{}
	if _, err := os.Stat(partialsPath); err == nil {
		partialsBytes, err := os.ReadFile(partialsPath)
		if err != nil {
			return fmt.Errorf("failed to read partials: %w", err)
		}
		p, err := yaml.Unmarshal[map[string]interface{}](partialsBytes)
		if err != nil {
			return fmt.Errorf("failed to unmarshal partials: %w", err)
		}
		partials = *p
	}

	// Capitalize keys of partials for template
	capitalizedPartials := make(map[string]interface{})
	for k, v := range partials {
		capitalizedPartials[strcase.ToCamel(k)] = v
	}

	// Prepare data for template
	distName := metadata.DistributionName
	distParts := strings.Split(distName, ":")
	groupId := ""
	artifactId := ""
	if len(distParts) > 0 {
		groupId = distParts[0]
	}
	if len(distParts) > 1 {
		artifactId = distParts[1]
	}

	repoName := metadata.Repo
	repoParts := strings.Split(repoName, "/")
	repoShort := ""
	if len(repoParts) > 0 {
		repoShort = repoParts[len(repoParts)-1]
	}

	version := libraryVersion

	minJavaVersion := metadata.MinJavaVersion
	if minJavaVersion == 0 {
		minJavaVersion = 8 // Default to Java 8
	}

	samples, err := ExtractSamples(dir)
	if err != nil {
		return fmt.Errorf("failed to extract samples: %w", err)
	}

	snippets, err := ExtractSnippets(dir)
	if err != nil {
		return fmt.Errorf("failed to extract snippets: %w", err)
	}

	templateMetadata := map[string]interface{}{
		"Repo":                metadata,
		"LibraryVersion":      version,
		"LibrariesBOMVersion": bomVersion,
		"Samples":             samples,
		"Snippets":            snippets,
		"MinJavaVersion":      minJavaVersion,
	}

	if len(capitalizedPartials) > 0 {
		templateMetadata["Partials"] = capitalizedPartials
	}

	data := struct {
		Metadata          map[string]interface{}
		GroupID           string
		ArtifactID        string
		Version           string
		RepoShort         string
		MigratedSplitRepo bool
		Monorepo          bool
		BOMVersion        string
		LibraryVersion    string
	}{
		Metadata:          templateMetadata,
		GroupID:           groupId,
		ArtifactID:        artifactId,
		Version:           version,
		RepoShort:         repoShort,
		MigratedSplitRepo: false,
		Monorepo:          true,
		BOMVersion:        bomVersion,
		LibraryVersion:    libraryVersion,
	}

	// Execute template
	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	// Write output
	return os.WriteFile(outputPath, []byte(buf.String()), 0644)
}
