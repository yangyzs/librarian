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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"text/template"

	"github.com/iancoleman/strcase"
	"gopkg.in/yaml.v3"
)

func isNilOrEmpty(v interface{}) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Map, reflect.Array, reflect.String:
		return rv.Len() == 0
	}
	return false
}

// RenderREADME renders the README.md file using the template and metadata.
// dir is the directory containing .repo-metadata.json and where README.md will be written.
// templatePath is the path to the README.md.go.tmpl file.
func RenderREADME(dir, templatePath, bomVersion, libraryVersion string) error {
	metadataPath := filepath.Join(dir, ".repo-metadata.json")
	partialsPath := filepath.Join(dir, ".readme-partials.yaml")
	if _, err := os.Stat(partialsPath); os.IsNotExist(err) {
		partialsPath = filepath.Join(dir, ".readme-partials.yml")
	}
	outputPath := filepath.Join(dir, "README.md")

	// Read metadata
	metadataBytes, err := os.ReadFile(metadataPath)
	if err != nil {
		return fmt.Errorf("failed to read metadata: %w", err)
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
		return fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	// Read partials if exist
	partials := make(map[string]interface{})
	if _, err := os.Stat(partialsPath); err == nil {
		partialsBytes, err := os.ReadFile(partialsPath)
		if err != nil {
			return fmt.Errorf("failed to read partials: %w", err)
		}
		if err := yaml.Unmarshal(partialsBytes, &partials); err != nil {
			return fmt.Errorf("failed to unmarshal partials: %w", err)
		}
	}

	// Capitalize keys of partials for template
	capitalizedPartials := make(map[string]interface{})
	for k, v := range partials {
		capitalizedPartials[strcase.ToCamel(k)] = v
	}

	// Prepare data for template
	distName, _ := metadata["distribution_name"].(string)
	if distName == "" {
		if repo, ok := metadata["repo"].(map[string]interface{}); ok {
			distName, _ = repo["distribution_name"].(string)
		}
	}
	distParts := strings.Split(distName, ":")
	groupID := ""
	artifactID := ""
	if len(distParts) > 0 {
		groupID = distParts[0]
	}
	if len(distParts) > 1 {
		artifactID = distParts[1]
	}

	repoName, _ := metadata["repo"].(string)
	if repoName == "" {
		if repo, ok := metadata["repo"].(map[string]interface{}); ok {
			repoName, _ = repo["repo"].(string)
		}
	}
	repoParts := strings.Split(repoName, "/")
	repoShort := ""
	if len(repoParts) > 0 {
		repoShort = repoParts[len(repoParts)-1]
	}

	version, _ := metadata["library_version"].(string)
	if version == "" {
		version = libraryVersion
	}

	// Construct the nested Metadata object for the template
	templateRepo := map[string]interface{}{
		"NamePretty":           metadata["name_pretty"],
		"DistributionName":     metadata["distribution_name"],
		"Repo":                 metadata["repo"],
		"APIShortname":         metadata["api_shortname"],
		"APIDescription":       metadata["api_description"],
		"ClientDocumentation":  metadata["client_documentation"],
		"ProductDocumentation": metadata["product_documentation"],
		"ReleaseLevel":         metadata["release_level"],
		"Transport":            metadata["transport"],
		"RequiresBilling":      metadata["requires_billing"],
		"APIID":                metadata["api_id"],
		"RepoShort":            metadata["repo_short"],
	}

	// If flat structure failed, try to get from nested repo
	if templateRepo["NamePretty"] == nil {
		if repo, ok := metadata["repo"].(map[string]interface{}); ok {
			templateRepo["NamePretty"] = repo["name_pretty"]
			templateRepo["DistributionName"] = repo["distribution_name"]
			templateRepo["Repo"] = repo["repo"]
			templateRepo["APIShortname"] = repo["api_shortname"]
			templateRepo["APIDescription"] = repo["api_description"]
			templateRepo["ClientDocumentation"] = repo["client_documentation"]
			templateRepo["ProductDocumentation"] = repo["product_documentation"]
			templateRepo["ReleaseLevel"] = repo["release_level"]
			templateRepo["Transport"] = repo["transport"]
			templateRepo["RequiresBilling"] = repo["requires_billing"]
			templateRepo["APIID"] = repo["api_id"]
			templateRepo["RepoShort"] = repo["repo_short"]
		}
	}

	minJavaVersion, _ := metadata["min_java_version"].(string)
	if minJavaVersion == "" {
		minJavaVersion = "8" // Default to Java 8
	}
	fmt.Println("DEBUG minJavaVersion:", minJavaVersion)

	samples := metadata["samples"]
	if isNilOrEmpty(samples) {
		extSamples, err := ExtractSamples(dir)
		if err != nil {
			return fmt.Errorf("failed to extract samples: %w", err)
		}
		if len(extSamples) > 0 {
			samples = extSamples
		}
	}

	snippets := metadata["snippets"]
	if isNilOrEmpty(snippets) {
		extSnippets, err := ExtractSnippets(dir)
		if err != nil {
			return fmt.Errorf("failed to extract snippets: %w", err)
		}
		if len(extSnippets) > 0 {
			snippets = extSnippets
		}
	}

	templateMetadata := map[string]interface{}{
		"Repo":                templateRepo,
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
		GroupID:           groupID,
		ArtifactID:        artifactID,
		Version:           version,
		RepoShort:         repoShort,
		MigratedSplitRepo: false,
		Monorepo:          true,
		BOMVersion:        bomVersion,
		LibraryVersion:    libraryVersion,
	}

	// Read and parse template
	tmplBytes, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("failed to read template: %w", err)
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
