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

package postprocessing

import (
	"errors"
	"testing"

	"github.com/googleapis/librarian/internal/config"
)

func TestValidate_Success(t *testing.T) {
	c := &config.Postprocess{
		Replace: []config.ReplaceConfig{
			{
				Path:        "path/to/file.java",
				Original:    "old string",
				Replacement: "new string",
			},
		},
		ReplaceRegex: []config.ReplaceRegexConfig{
			{
				Path:        "path/to/file.java",
				Pattern:     "pattern",
				Replacement: "replacement",
			},
		},
		CopyFile: []config.CopyConfig{
			{
				Src: "path/to/src.java",
				Dst: "path/to/dst.java",
			},
		},
		RemoveFile: []string{"path/to/file_to_remove.java"},
		MethodOperations: []config.MethodOperation{
			{
				Path:     "path/to/file.java",
				Action:   "delete",
				FuncName: "public void toDelete()",
			},
			{
				Path:     "path/to/file.java",
				Action:   "duplicate",
				FuncName: "public void toDuplicate()",
				NewName:  "duplicated",
			},
			{
				Path:               "path/to/file.java",
				Action:             "deprecate",
				FuncName:           "public void toDeprecate()",
				DeprecationMessage: "Use alternative instead.",
			},
		},
	}

	if err := Validate(c); err != nil {
		t.Fatalf("Validate() expected no error, got: %v", err)
	}
}

func TestValidate_Errors(t *testing.T) {
	for _, test := range []struct {
		name    string
		config  *config.Postprocess
		wantErr error
	}{
		{
			name: "invalid signature for delete",
			config: &config.Postprocess{
				MethodOperations: []config.MethodOperation{
					{
						Path:     "path/to/file.java",
						Action:   "delete",
						FuncName: "invalidSignature",
					},
				},
			},
			wantErr: errInvalidSignature,
		},
		{
			name: "invalid signature for duplicate",
			config: &config.Postprocess{
				MethodOperations: []config.MethodOperation{
					{
						Path:     "path/to/file.java",
						Action:   "duplicate",
						FuncName: "invalidSignature",
						NewName:  "foo",
					},
				},
			},
			wantErr: errInvalidSignature,
		},
		{
			name: "missing new name for duplicate",
			config: &config.Postprocess{
				MethodOperations: []config.MethodOperation{
					{
						Path:     "path/to/file.java",
						Action:   "duplicate",
						FuncName: "void foo()",
						NewName:  "",
					},
				},
			},
			wantErr: errEmptyNewName,
		},
		{
			name: "invalid signature for deprecate",
			config: &config.Postprocess{
				MethodOperations: []config.MethodOperation{
					{
						Path:               "path/to/file.java",
						Action:             "deprecate",
						FuncName:           "invalidSignature",
						DeprecationMessage: "foo",
					},
				},
			},
			wantErr: errInvalidSignature,
		},
		{
			name: "missing message for deprecate",
			config: &config.Postprocess{
				MethodOperations: []config.MethodOperation{
					{
						Path:               "path/to/file.java",
						Action:             "deprecate",
						FuncName:           "void foo()",
						DeprecationMessage: "",
					},
				},
			},
			wantErr: errEmptyDeprecationMessage,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := Validate(test.config)
			if !errors.Is(err, test.wantErr) {
				t.Errorf("Validate() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}
