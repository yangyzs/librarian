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
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/googleapis/librarian/internal/yaml"
	"github.com/iancoleman/strcase"
)

//go:embed template/README.md.go.tmpl
var defaultTemplateFs embed.FS

// RenderREADME renders the README.md file using the template and metadata.
// dir is the directory containing where README.md will be written.
func RenderREADME(dir string, metadata *repoMetadata, bomVersion, libraryVersion string, keepSet map[string]bool) error {
	outputPath := filepath.Join(dir, "README.md")
	if isKept("README.md", keepSet) {
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

	// Read and parse template from disk
	templatePath := filepath.Join(dir, "template", "README.md.go.tmpl")
	tmplBytes, err := os.ReadFile(templatePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// Fallback to embedded default template
			tmplBytes, err = defaultTemplateFs.ReadFile("template/README.md.go.tmpl")
		}
		if err != nil {
			return fmt.Errorf("failed to read template: %w", err)
		}
	}

	tmpl, err := template.New("README").Parse(string(tmplBytes))
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	// Execute template
	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	// Write output
	return os.WriteFile(outputPath, []byte(buf.String()), 0644)
}
