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
	"bytes"
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
	"unicode"

	"github.com/googleapis/librarian/internal/yaml"
)

var (
	//go:embed template/README.md.go.tmpl
	readmeTmpl        string
	readmeTmplParsed  = template.Must(template.New("README").Parse(readmeTmpl))
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

	// errEmptyDir indicates an empty directory string was provided.
	errEmptyDir = errors.New("dir cannot be empty")

	// errEmptyFile indicates an empty file path was provided.
	errEmptyFile = errors.New("file cannot be empty")

	// errNilMetadata indicates a nil repoMetadata pointer was provided.
	errNilMetadata = errors.New("metadata cannot be nil")
)

// codeSample represents a discovered Java code sample along with its derived title.
type codeSample struct {
	Title string
	File  string
}

// readmeData represents the top-level template execution context passed to README.md.go.tmpl.
type readmeData struct {
	Metadata          map[string]interface{} // Contains Repo, LibraryVersion, Samples, Snippets, Partials. // TODO delete these comments for readmeData
	GroupID           string                 // Maven Group ID (e.g. com.google.cloud), required for Maven/Gradle dependency blocks.
	ArtifactID        string                 // Maven Artifact ID (e.g. google-cloud-storage), required for dependency blocks.
	Version           string                 // Current library version.
	RepoShort         string                 // Short repository name used in GitHub archive migration notices.
	MigratedSplitRepo bool                   // Flag indicating if repository moved to monorepo.
	Monorepo          bool                   // Flag indicating if library is part of google-cloud-java monorepo.
	BOMVersion        string                 // Version of libraries-bom for dependencyManagement block.
}

// renderREADME generates README.md in dir using the embedded Markdown template.
// It injects repository metadata, versions, samples, and snippets, skipping rendering if protected by keepSet.
func renderREADME(dir string, metadata *repoMetadata, bomVersion, libraryVersion string, keepSet map[string]bool) error {
	if dir == "" {
		return errEmptyDir
	}
	if metadata == nil {
		return errNilMetadata
	}
	if keepSet["README.md"] {
		return nil
	}
	partials, err := loadReadmePartials(dir)
	if err != nil {
		return err
	}
	groupID, artifactID := parseGroupIDArtifactID(metadata.DistributionName)
	repoShort := parseRepoShortName(metadata.Repo)
	minJavaVersion := metadata.MinJavaVersion
	if minJavaVersion == 0 {
		minJavaVersion = 8
	}
	samples, err := extractSamples(dir)
	if err != nil {
		return fmt.Errorf("failed to extract samples: %w", err)
	}
	snippets, err := extractSnippets(dir)
	if err != nil {
		return fmt.Errorf("failed to extract snippets: %w", err)
	}
	templateMetadata := map[string]interface{}{
		"Repo":           metadata,
		"Samples":        samples,
		"Snippets":       snippets,
		"MinJavaVersion": minJavaVersion,
	}
	if len(partials) > 0 {
		templateMetadata["Partials"] = partials
	}
	data := readmeData{
		Metadata:          templateMetadata,
		GroupID:           groupID,
		ArtifactID:        artifactID,
		Version:           libraryVersion,
		RepoShort:         repoShort,
		MigratedSplitRepo: false,
		Monorepo:          true,
		BOMVersion:        bomVersion,
	}
	var buf strings.Builder
	if err := readmeTmplParsed.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}
	outputPath := filepath.Join(dir, "README.md")
	return os.WriteFile(outputPath, []byte(buf.String()), 0644)
}

// extractSamples locates standard Java example files under the "samples" directory.
// It returns a codeSample struct containing the display title and relative path of each sample for README rendering.
func extractSamples(dir string) ([]codeSample, error) {
	if dir == "" {
		return nil, errEmptyDir
	}
	files, err := collectSampleFiles(dir)
	if err != nil {
		return nil, err
	}
	var samples []codeSample
	for _, file := range files {
		sample, err := parseCodeSample(dir, file)
		if err != nil {
			return nil, err
		}
		samples = append(samples, *sample)
	}
	return samples, nil
}

// collectSampleFiles recursively scans dir/samples for Java production files.
func collectSampleFiles(dir string) ([]string, error) {
	samplesDir := filepath.Join(dir, "samples")
	if _, err := os.Stat(samplesDir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to stat samples directory: %w", err)
	}
	var files []string
	err := filepath.WalkDir(samplesDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
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
	return files, nil
}

// parseCodeSample reads a Java sample file and constructs a codeSample struct with its title and relative path.
func parseCodeSample(dir, file string) (*codeSample, error) {
	// Derive default title by stripping extension and converting CamelCase to space-separated words.
	base := strings.TrimSuffix(filepath.Base(file), ".java")
	title := decamelize(base)
	titleOverride, err := extractTitle(filepath.Join(dir, file))
	if err != nil {
		return nil, fmt.Errorf("failed to extract title from %s: %w", file, err)
	}
	if titleOverride != "" {
		title = titleOverride
	}
	return &codeSample{
		Title: title,
		// Normalize path separators to forward slashes for Markdown links in README.
		File: filepath.ToSlash(file),
	}, nil
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

// extractTitle reads a file from disk and extracts the title override from Java comment blocks.
// It expects a "title:" line to immediately follow the "sample-metadata:" marker.
// Returns an error if the file cannot be read, or if the marker is present but the title line
// is missing, malformed, or empty.
func extractTitle(filePath string) (string, error) {
	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	if !bytes.Contains(contentBytes, []byte("sample-metadata:")) {
		return "", nil
	}
	content := string(contentBytes)
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

// extractSnippets walks the "samples" directory locating *.java and *.xml files.
// It line-scans for START and END tags while supporting START_EXCLUDE blocks.
func extractSnippets(dir string) (map[string]string, error) {
	if dir == "" {
		return nil, errEmptyDir
	}
	files, err := collectSnippetFiles(dir)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, nil
	}
	sort.Strings(files) // TODO: check- do we need this? check the legacy owlbot and see if we sort the files?
	snippetLines := make(map[string][]string)
	for _, file := range files {
		fileSnippets, err := extractSnippetsFromFile(file)
		if err != nil {
			return nil, err
		}
		for name, lines := range fileSnippets {
			snippetLines[name] = append(snippetLines[name], lines...)
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

// collectSnippetFiles recursively scans dir/samples for Java and XML files containing snippets.
func collectSnippetFiles(dir string) ([]string, error) {
	samplesDir := filepath.Join(dir, "samples")
	if _, err := os.Stat(samplesDir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to stat samples directory: %w", err)
	}
	var files []string
	err := filepath.WalkDir(samplesDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "test" || (d.Name() == "generated" && filepath.Base(filepath.Dir(path)) == "snippets") {
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
		return nil, fmt.Errorf("failed to walk samples directory: %w", err)
	}
	return files, nil
}

// extractSnippetsFromFile parses a single file to return a map of tagged code snippets.
// Reads code between START and END markers while omitting EXCLUDE blocks.
// START and END markers must be named (e.g. [START <name>]).
func extractSnippetsFromFile(file string) (map[string][]string, error) {
	if file == "" {
		return nil, errEmptyFile
	}
	f, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", file, err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	// 10 MB sanity limit to protect system memory.
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)
	// Scan through file line by line and capture all open snippets.
	// More than one open block might exist for a given line, so we
	// need to track open snippets in a map.
	snippetLines := make(map[string][]string)
	openSnippets := make(map[string]bool)
	excluding := false
	for scanner.Scan() {
		line := scanner.Text()
		if openExcludeRegex.MatchString(line) {
			excluding = true
			continue
		}
		if closeExcludeRegex.MatchString(line) {
			excluding = false
			continue
		}
		if excluding {
			continue
		}
		openMatch := openSnippetRegex.FindStringSubmatch(line)
		closeMatch := closeSnippetRegex.FindStringSubmatch(line)
		if len(openMatch) > 1 {
			name := openMatch[1]
			openSnippets[name] = true
			if _, exists := snippetLines[name]; !exists {
				snippetLines[name] = []string{}
			}
			continue
		}
		if len(closeMatch) > 1 {
			delete(openSnippets, closeMatch[1])
			continue
		}
		for s := range openSnippets {
			snippetLines[s] = append(snippetLines[s], line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed scanning file %s: %w", file, err)
	}
	return snippetLines, nil
}

// minLeadingSpaces finds the minimum number of leading spaces across non-empty lines.
func minLeadingSpaces(lines []string) int {
	if len(lines) == 0 {
		return 0
	}
	minSpaces := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		spaces := len(line) - len(strings.TrimLeft(line, " "))
		if minSpaces == -1 || spaces < minSpaces {
			minSpaces = spaces
		}
	}
	if minSpaces == -1 {
		return 0
	}
	return minSpaces
}

// trimLeadingWhitespace computes minimum leading space indentation and trims it.
// Used to clean up snippet lines so code formatting looks natural in README blocks.
func trimLeadingWhitespace(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	minSpaces := minLeadingSpaces(lines)
	var sb strings.Builder
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			sb.WriteString("\n")
			continue
		}
		if len(line) >= minSpaces {
			sb.WriteString(line[minSpaces:])
		} else {
			sb.WriteString(line)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// loadReadmePartials loads and camel-cases README partials from .readme-partials.yaml or .yml.
func loadReadmePartials(dir string) (map[string]interface{}, error) {
	if dir == "" {
		return nil, errEmptyDir
	}
	partialsPath := filepath.Join(dir, ".readme-partials.yaml")
	if _, err := os.Stat(partialsPath); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("failed to stat partials file: %w", err)
		}
		partialsPath = filepath.Join(dir, ".readme-partials.yml")
		if _, err = os.Stat(partialsPath); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil, nil
			}
			return nil, fmt.Errorf("failed to stat partials file: %w", err)
		}
	}
	partialsBytes, err := os.ReadFile(partialsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read partials file: %w", err)
	}
	if partialsBytes == nil {
		return nil, nil
	}
	rawPartials, err := yaml.Unmarshal[map[string]interface{}](partialsBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal partials: %w", err)
	}
	result := make(map[string]interface{})
	for k, v := range *rawPartials {
		// Convert to camel case because jinja templates use snake_case but go template fields should use camelcase.
		result[toCamelCase(k)] = v
	}
	return result, nil
}

// toCamelCase converts snake_case, kebab-case, or lower word strings to CamelCase.
func toCamelCase(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == '-' || r == ' '
	})
	var sb strings.Builder
	for _, p := range parts {
		if len(p) > 0 {
			r := []rune(p)
			r[0] = unicode.ToUpper(r[0])
			sb.WriteString(string(r))
		}
	}
	return sb.String()
}

// parseGroupIDArtifactID extracts GroupID and ArtifactID from a Maven distribution name.
func parseGroupIDArtifactID(distributionName string) (string, string) {
	groupID, artifactID, _ := strings.Cut(distributionName, ":")
	return groupID, artifactID
}

// parseRepoShortName extracts the short repository name from the full repo path.
func parseRepoShortName(repo string) string {
	if repo == "" {
		return ""
	}
	if i := strings.LastIndexByte(repo, '/'); i >= 0 {
		return repo[i+1:]
	}
	return repo
}
