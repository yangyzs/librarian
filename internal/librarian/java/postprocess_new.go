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
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/googleapis/librarian/internal/filesystem"
	"github.com/googleapis/librarian/internal/postprocessing"
)

// postProcessLibraryNew implements the new postprocessing flow, bypassing owlbot.py.
// It applies post-processing operations configured in librarian.yaml, and renders the README.md directly
// on the generated files in their final destinations.
func postProcessLibraryNew(ctx context.Context, p libraryPostProcessParams) error {
	keepSet := make(map[string]bool, len(p.library.Keep))
	for _, k := range p.library.Keep {
		normalized := strings.TrimSuffix(filepath.ToSlash(k), "/")
		keepSet[normalized] = true
	}

	// 1. Load postprocess configuration and apply operations
	if p.library.Postprocess != nil {
		if err := postprocessing.Validate(p.library.Postprocess); err != nil {
			return fmt.Errorf("invalid postprocess config: %w", err)
		}
		cfg := p.library.Postprocess

		// 1. Apply Copies
		for _, c := range cfg.CopyFile {
			if keepSet[filepath.ToSlash(c.Dst)] {
				continue
			}
			srcAbs := filepath.Join(p.outDir, c.Src)
			dstAbs := filepath.Join(p.outDir, c.Dst)
			if err := filesystem.CopyFile(srcAbs, dstAbs); err != nil {
				return fmt.Errorf("failed to copy file from %s to %s: %w", c.Src, c.Dst, err)
			}
		}

		// 2. Apply Removes
		for _, rem := range cfg.RemoveFile {
			if err := applyToFiles(p.outDir, rem, func(file string) error {
				if err := postprocessing.RemoveFile(file); err != nil {
					return fmt.Errorf("failed to remove file %s: %w", file, err)
				}
				return nil
			}); err != nil {
				return err
			}
		}

		// 3. Apply Replacements
		for _, r := range cfg.Replace {
			if err := applyToFiles(p.outDir, r.Path, func(file string) error {
				if err := postprocessing.Replace(file, r.Original, r.Replacement); err != nil {
					return fmt.Errorf("failed to apply replacement in %s: %w", file, err)
				}
				return nil
			}); err != nil {
				return err
			}
		}

		// 4. Apply Regex Replacements
		for _, r := range cfg.ReplaceRegex {
			if err := applyToFiles(p.outDir, r.Path, func(file string) error {
				if err := postprocessing.ReplaceRegex(file, r.Pattern, r.Replacement); err != nil {
					return fmt.Errorf("failed to apply regex replacement in %s: %w", file, err)
				}
				return nil
			}); err != nil {
				return err
			}
		}

		// 5. Apply Method Operations
		for _, mo := range cfg.MethodOperations {
			if err := applyToFiles(p.outDir, mo.Path, func(file string) error {
				switch mo.Action {
				case "delete":
					if err := postprocessing.DeleteMethod(file, mo.FuncName, "java"); err != nil {
						return fmt.Errorf("failed to delete method %q in %s: %w", mo.FuncName, file, err)
					}
				case "duplicate":
					if err := postprocessing.DuplicateMethod(ctx, file, mo.FuncName, mo.NewName, "java"); err != nil {
						return fmt.Errorf("failed to duplicate method %q in %s: %w", mo.FuncName, file, err)
					}
				case "deprecate":
					if err := postprocessing.DeprecateMethod(file, mo.FuncName, mo.DeprecationMessage, "java"); err != nil {
						return fmt.Errorf("failed to deprecate method %q in %s: %w", mo.FuncName, file, err)
					}
				default:
					return fmt.Errorf("unsupported method operation action %q", mo.Action)
				}
				return nil
			}); err != nil {
				return err
			}
		}
	}

	// 6. Render README.md

	var libraryVersion string
	if p.library.Java != nil && p.library.Java.ReleasedVersion != "" {
		libraryVersion = p.library.Java.ReleasedVersion
	} else {
		var err error
		libraryVersion, err = deriveLastReleasedVersion(p.library.Version)
		if err != nil {
			return fmt.Errorf("failed to derive library version: %w", err)
		}
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

	bomVersion, err := findBOMVersion(p.cfg, p.library)
	if err != nil {
		return fmt.Errorf("failed to find BOM version: %w", err)
	}

	if err := RenderREADME(p.outDir, p.metadata, bomVersion, libraryVersion, keepSet); err != nil {
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

func applyToFiles(outDir string, pathPattern string, action func(string) error) error {
	files, err := resolveGlobs(outDir, pathPattern)
	if err != nil {
		return fmt.Errorf("failed to resolve glob for %s: %w", pathPattern, err)
	}
	isGlob := strings.ContainsAny(pathPattern, "*?[]{}")
	var replacedAny bool
	var lastTextNotFoundErr error
	for _, file := range files {
		if err := action(file); err != nil {
			if isGlob && errors.Is(err, postprocessing.ErrTextNotFound) {
				lastTextNotFoundErr = err
				continue
			}
			return err
		}
		replacedAny = true
	}
	if isGlob && len(files) > 0 && !replacedAny {
		return lastTextNotFoundErr
	}
	return nil
}
