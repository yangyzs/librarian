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
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/googleapis/librarian/internal/filesystem"
	"github.com/googleapis/librarian/internal/postprocessing"
)

// postProcessLibraryNew implements the new postprocessing flow, bypassing owlbot.py.
// It applies operations from postprocess.yaml, and renders the README.md directly
// on the generated files in their final destinations.
func postProcessLibraryNew(p libraryPostProcessParams) error {
	// 1. Load postprocess.yaml and apply operations
	postprocessYamlPath := filepath.Join(p.outDir, "postprocess.yaml")
	if _, err := os.Stat(postprocessYamlPath); err == nil {
		cfg, err := postprocessing.ParseConfig(postprocessYamlPath)
		if err != nil {
			return fmt.Errorf("failed to parse postprocess.yaml: %w", err)
		}

		// 1. Apply Copies
		for _, c := range cfg.CopyFile {
			srcAbs := filepath.Join(p.outDir, c.Src)
			dstAbs := filepath.Join(p.outDir, c.Dst)
			if err := filesystem.CopyFile(srcAbs, dstAbs); err != nil {
				return fmt.Errorf("failed to copy file from %s to %s: %w", c.Src, c.Dst, err)
			}
		}

		// 2. Apply Removes
		for _, rem := range cfg.RemoveFile {
			absPath := filepath.Join(p.outDir, rem)
			if err := postprocessing.RemoveFile(absPath); err != nil {
				return fmt.Errorf("failed to remove file %s: %w", rem, err)
			}
		}

		// 3. Apply Replacements
		for _, r := range cfg.Replace {
			absPath := filepath.Join(p.outDir, r.Path)
			if err := postprocessing.Replace(absPath, r.Original, r.Replacement); err != nil {
				return fmt.Errorf("failed to apply replacement in %s: %w", r.Path, err)
			}
		}

		// 4. Apply Regex Replacements
		for _, r := range cfg.ReplaceRegex {
			absPath := filepath.Join(p.outDir, r.Path)
			if err := postprocessing.ReplaceRegex(absPath, r.Pattern, r.Replacement); err != nil {
				return fmt.Errorf("failed to apply regex replacement in %s: %w", r.Path, err)
			}
		}

		// 5. Apply Method Operations
		for _, mo := range cfg.MethodOperations {
			files, err := resolveGlobs(p.outDir, mo.Path)
			if err != nil {
				return fmt.Errorf("failed to resolve glob for %s: %w", mo.Path, err)
			}
			for _, file := range files {
				switch mo.Action {
				case "delete":
					if err := postprocessing.DeleteFunc(file, mo.FuncName, "java"); err != nil {
						if strings.Contains(err.Error(), "not found") {
							continue
						}
						return fmt.Errorf("failed to delete method %q in %s: %w", mo.FuncName, file, err)
					}
				case "duplicate":
					if err := postprocessing.DuplicateMethod(file, mo.FuncName, mo.NewName, "java"); err != nil {
						if strings.Contains(err.Error(), "not found") {
							continue
						}
						return fmt.Errorf("failed to duplicate method %q in %s: %w", mo.FuncName, file, err)
					}
				case "deprecate":
					if err := postprocessing.DeprecateMethod(file, mo.FuncName, mo.DeprecationMessage, "java"); err != nil {
						if strings.Contains(err.Error(), "not found") {
							continue
						}
						return fmt.Errorf("failed to deprecate method %q in %s: %w", mo.FuncName, file, err)
					}
				default:
					return fmt.Errorf("unsupported method operation action %q", mo.Action)
				}
			}
		}
	}

	// 4. Render README.md
	templatePath := filepath.Join("template", "README.md.go.tmpl")
	if _, err := os.Stat(templatePath); err != nil {
		// Fallback to absolute path for this workspace
		templatePath = "/usr/local/google/home/sophieeee/workspace/owlbot-modernization/librarian/internal/librarian/java/template/README.md.go.tmpl"
	}

	libraryVersion, err := deriveLastReleasedVersion(p.library.Version)
	if err != nil {
		return fmt.Errorf("failed to derive library version: %w", err)
	}

	if p.cfg == nil {
		return fmt.Errorf("cfg is nil")
	}
	if p.cfg.Default == nil {
		return fmt.Errorf("cfg.Default is nil")
	}
	if p.cfg.Default.Java == nil {
		return fmt.Errorf("cfg.Default.Java is nil")
	}

	bomVersion, err := findBOMVersion(p.cfg)
	if err != nil {
		return fmt.Errorf("failed to find BOM version: %w", err)
	}

	if err := RenderREADME(p.outDir, templatePath, bomVersion, libraryVersion); err != nil {
		return fmt.Errorf("failed to render README: %w", err)
	}

	return nil
}

func resolveGlobs(outDir, pathPattern string) ([]string, error) {
	if strings.ContainsAny(pathPattern, "*?[]{}") {
		return doublestar.FilepathGlob(filepath.Join(outDir, pathPattern))
	}
	return []string{filepath.Join(outDir, pathPattern)}, nil
}
