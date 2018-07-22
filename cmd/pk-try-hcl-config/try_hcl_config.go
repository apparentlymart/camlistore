// pk-try-hcl-config is a temporary command for exercising the HCL config
// proof-of-concept code. It is not intended as something to ever include
// in a Perkeep release.
package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/davecgh/go-spew/spew"
	"github.com/hashicorp/hcl2/hcl"

	"perkeep.org/pkg/blobserver/s3"
	"perkeep.org/pkg/configs"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal("Wrong number of args")
	}

	filename := os.Args[1]

	src, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Fatal(err)
	}

	config, f, err := configs.ReadConfig(src, filename)
	if err != nil {
		switch tErr := err.(type) {
		case hcl.Diagnostics:
			files := map[string]*hcl.File{
				filename: f,
			}
			wr := hcl.NewDiagnosticTextWriter(os.Stderr, files, 78, true)
			wr.WriteDiagnostics(tErr)
			os.Exit(1)
		default:
			log.Fatal(err)
		}
	}

	fmt.Println("")

	if c := config.Network; c != nil {
		fmt.Printf("# Network Configuration\n\n")
		fmt.Printf("* Base URL: `%s`\n", c.BaseURL)
		fmt.Printf("* Listen Address: `%s`\n", c.ListenAddr)
		fmt.Printf("* HTTPS enabled: %t\n", c.HTTPS)
		fmt.Printf("* Auths:\n")
		for _, c := range c.Auths {
			fmt.Printf("  * `%s`\n", c.Type)
		}
		fmt.Println("")
	}

	if c := config.Identity; c != nil {
		fmt.Printf("# Identity Configuration\n\n")
		fmt.Printf("* ID: %s\n", c.ID)
		fmt.Printf("* Keyring Path: `%s`\n", c.KeyringPath)
		fmt.Println("")
	}

	if c := config.Server; c != nil {
		fmt.Printf("# Server Configuration\n\n")
		fmt.Printf("* Search Handler enabled: %t\n", c.SearchHandler)
		fmt.Printf("* Share Handler enabled: %t\n", c.ShareHandler)
		fmt.Printf("* UI Handler enabled: %t\n", c.UIHandler)
		fmt.Printf("* Blobs Write To:\n")
		for _, c := range c.BlobWrites {
			fmt.Printf("  * `%s`\n", c)
		}
		fmt.Println("")
	}

	if c := config.Index; c != nil {
		fmt.Printf("# Index Configuration\n\n")
		fmt.Printf("* Type: `%s`\n", c.Type)
		fmt.Println("")
	}

	if c := config.Stores; len(c) > 0 {
		fmt.Printf("# Storage Handlers\n\n")
		for _, c := range c {
			fmt.Printf("* `%s`\n", c.Type)
		}
		fmt.Println("")
	}

	if c := config.Syncs; len(c) > 0 {
		fmt.Printf("# Syncs\n\n")
		for _, c := range c {
			fmt.Printf("* From `%s` to `%s`\n", c.From, c.To)
		}
		fmt.Println("")
	}

	// Since this prototype doesn't wire up HCL-based loaders for all of the
	// blobstorage implementations, we just handle "s3" manually here to see
	// how an HCL-based loader for that might behave. In practice, this
	// would presumably be accessed via the blobserver store registry,
	// along with all of the other implementations.
	if c, ok := config.Stores["s3"]; ok {
		store, err := s3.NewFromHCLConfig(nil, c)
		if err != nil {
			switch tErr := err.(type) {
			case hcl.Diagnostics:
				files := map[string]*hcl.File{
					filename: f,
				}
				wr := hcl.NewDiagnosticTextWriter(os.Stderr, files, 78, true)
				wr.WriteDiagnostics(tErr)
				os.Exit(1)
			default:
				log.Fatal(err)
			}
		}

		fmt.Printf("# S3 Blob Storage\n\n")
		fmt.Printf("```\n%s\n````\n\n", spew.Sdump(store))
	}
}
