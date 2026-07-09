package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/Silo-Community/silo-plugins/catalog"
)

func main() {
	var manifestPath string
	var approvalsPath string
	flag.StringVar(&manifestPath, "manifest", "manifest.json", "Path to the catalog manifest")
	flag.StringVar(&approvalsPath, "approvals", "approved-plugins.json", "Path to the approved plugin registry")
	flag.Parse()

	approvalsData, err := os.ReadFile(approvalsPath)
	if err != nil {
		exitf("read approvals: %v", err)
	}
	registry, err := catalog.DecodeApprovalRegistry(approvalsData)
	if err != nil {
		exitf("decode approvals: %v", err)
	}

	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		exitf("read catalog: %v", err)
	}
	var index catalog.RepositoryIndex
	if err := json.Unmarshal(manifestData, &index); err != nil {
		exitf("decode catalog: %v", err)
	}
	if err := catalog.ValidateApprovedIndex(index, registry); err != nil {
		exitf("validate catalog: %v", err)
	}
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
