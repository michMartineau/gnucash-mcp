package main

import (
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/server"

	"github.com/michelgermain/gnucash-mcp/internal/gnucash"
	"github.com/michelgermain/gnucash-mcp/tools"
)

func main() {
	filepath := os.Getenv("GNUCASH_FILE")
	if filepath == "" {
		fmt.Fprintln(os.Stderr, "GNUCASH_FILE environment variable is required")
		fmt.Fprintln(os.Stderr, "Set it to the path of your GnuCash SQLite file")
		os.Exit(1)
	}

	db, err := gnucash.NewDB(filepath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open GnuCash database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	svc := gnucash.NewService(db)

	s := server.NewMCPServer(
		"gnucash",
		"1.0.0",
		server.WithToolCapabilities(false),
	)

	tools.RegisterTools(s, svc)

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
