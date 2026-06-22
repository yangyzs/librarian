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
	"bufio"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/googleapis/librarian/internal/yaml"
)

var (
	openSnippetRegex  = regexp.MustCompile(`\[START ([a-zA-Z0-9_-]+)\]`)
	closeSnippetRegex = regexp.MustCompile(`\[END ([a-zA-Z0-9_-]+)\]`)
	openExcludeRegex  = regexp.MustCompile(`\[START_EXCLUDE\]`)
	closeExcludeRegex = regexp.MustCompile(`\[END_EXCLUDE\]`)

	reMetadataBlock = regexp.MustCompile(`(?m)^[ \t]*//[ \t]*sample-metadata:([^\n]+|\n[ \t]*//)+`)
	reCommentPrefix = regexp.MustCompile(`(?m)^[ \t]*(?:#|//)[ \t]?`)

	reDecamelize1 = regexp.MustCompile(`([A-Z]+)([A-Z])([a-z0-9])`)
	reDecamelize2 = regexp.MustCompile(`([a-z0-9])([A-Z])`)
)

// decamelize converts CamelCase or PascalCase titles into space-separated words,
// exactly reproducing Python synthtool's _decamelize(value: str).
func decamelize(value string) string {
	if value == "" {
		return ""
	}
	r := []rune(value)
	r[0] = unicode.ToUpper(r[0])
	s := string(r)

	s = reDecamelize1.ReplaceAllString(s, "${1} ${2}${3}")
	return reDecamelize2.ReplaceAllString(s, "${1} ${2}")
}

// ExtractSamples walks the "samples" directory locating all .java source files.
// It parses embedded multiline "// sample-metadata:" YAML blocks to derive title and path metadata.
func ExtractSamples(dir string) ([]map[string]interface{}, error) {
	samplesDir := filepath.Join(dir, "samples")
	var files []string

	err := filepath.WalkDir(samplesDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) || os.IsNotExist(err) {
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
		if !d.IsDir() && d.Type().IsRegular() && filepath.Ext(path) == ".java" && strings.Contains(filepath.ToSlash(path), "/src/main/java/") {
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
	var samples []map[string]interface{}

	for _, file := range files {
		rel, err := filepath.Rel(dir, file)
		if err != nil {
			continue
		}
		base := strings.TrimSuffix(filepath.Base(file), ".java")
		title := decamelize(base)

		slashPath := filepath.ToSlash(rel)
		item := map[string]interface{}{
			"Title": title,
			"File":  slashPath,
			"title": title,
			"file":  slashPath,
		}

		contentBytes, err := os.ReadFile(file)
		if err != nil {
			samples = append(samples, item)
			continue
		}
		match := reMetadataBlock.FindString(string(contentBytes))
		if match == "" {
			samples = append(samples, item)
			continue
		}
		cleaned := reCommentPrefix.ReplaceAllString(match, "")
		meta, err := yaml.Unmarshal[map[string]map[string]interface{}]([]byte(cleaned))
		if err != nil {
			samples = append(samples, item)
			continue
		}
		sm, ok := (*meta)["sample-metadata"]
		if !ok {
			samples = append(samples, item)
			continue
		}
		for k, v := range sm {
			item[k] = v
			if len(k) > 0 {
				upperKey := strings.ToUpper(k[:1]) + k[1:]
				item[upperKey] = v
			}
		}
		if t, ok := sm["title"].(string); ok && strings.TrimSpace(t) != "" {
			item["Title"] = t
			item["title"] = t
		}
		samples = append(samples, item)
	}
	return samples, nil
}

// ExtractSnippets walks the "samples" directory locating *.java and *.xml files.
// It line-scans for [START name] and [END name] tags while supporting [START_EXCLUDE] blocks,
// returning trimmed minimum plain-space indentation blocks.
func ExtractSnippets(dir string) (map[string]string, error) {
	samplesDir := filepath.Join(dir, "samples")
	var files []string

	err := filepath.WalkDir(samplesDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) || os.IsNotExist(err) {
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
		f, err := os.Open(file)
		if err != nil {
			continue
		}

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
			f.Close()
			return nil, err
		}
		f.Close()
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
