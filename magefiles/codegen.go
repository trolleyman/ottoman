//go:build mage

package main

import (
	"fmt"
	"os"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

type Generate mg.Namespace

// All runs both Go and TypeScript generation
func (Generate) All() {
	mg.Deps(Generate.Go, Generate.TypeScript)
}

// Go generates the Go server interface and types
func (Generate) Go() error {
	fmt.Println("🚀 Generating Go API...")

	// Ensure internal/api exists
	if err := os.MkdirAll("internal/api", 0755); err != nil {
		return err
	}

	// Generate the Server Interface and Types
	// We use "server" generation because both the Controller (Pi)
	// and Agent (Desktop) implement this API to some degree.
	return sh.Run("oapi-codegen",
		"-package", "api",
		"-generate", "types,server,spec",
		"-o", "internal/api/server.gen.go",
		"api/openapi.yaml",
	)
}

// TypeScript generates the React client
func (Generate) TypeScript() error {
	fmt.Println("🚀 Generating TypeScript Client...")

	// Ensure web/src/api exists
	if err := os.MkdirAll("web/src/api", 0755); err != nil {
		return err
	}

	// uses openapi-typescript-codegen
	// --client fetch: Uses the native Fetch API (lightweight, no axios)
	// --name OttomanClient: The name of the client class
	return sh.Run("bun", "x", "openapi-typescript-codegen",
		"--input", "api/openapi.yaml",
		"--output", "web/src/api",
		"--client", "fetch",
		"--name", "OttomanClient",
	)
}

// Docs generates AsyncAPI documentation (Optional)
func (Generate) Docs() error {
	// Requires: npm install -g @asyncapi/generator
	return sh.Run("bun", "x", "ag", "api/asyncapi.yaml", "@asyncapi/html-template", "-o", "docs/asyncapi")
}
