// Copyright (c) daroco
// SPDX-License-Identifier: MPL-2.0

//go:build tools

// Package tools pins the code-generation tooling (tfplugindocs) as an explicit
// module dependency so `make docs` uses a reproducible version. It is never
// compiled into the provider binary (the `tools` build tag excludes it).
package tools

import (
	// Terraform Registry documentation generator.
	_ "github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs"
)

// Generate the docs/ tree the Terraform Registry serves, from the provider
// schema and the examples/ files. Requires the Terraform CLI on PATH.
//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate --provider-dir .. -provider-name satisfactory
