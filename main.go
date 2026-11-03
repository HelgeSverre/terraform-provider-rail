package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/HelgeSverre/terraform-provider-rail/internal/provider"
)

// Set by GoReleaser.
var version = "dev"

//go:generate go tool tfplugindocs generate --provider-name rail

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run with support for debuggers like delve")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/helgesverre/rail",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err)
	}
}
