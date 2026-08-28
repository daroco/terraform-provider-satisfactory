// mockserver runs the in-memory mock of the SatisfactoTerraform mod API for
// local provider development: `go run ./cmd/mockserver` then `terraform apply`.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/daroco/terraform-provider-satisfactory/internal/mockserver"
)

func main() {
	addr := flag.String("addr", ":8090", "listen address")
	seed := flag.Bool("seed", false, "populate a small hand-built factory and a player, so `satisfactory-export` has something to find")
	flag.Parse()

	srv := mockserver.New(os.Getenv("SATISFACTORY_TOKEN"))
	if *seed {
		srv.Seed(mockserver.SampleHandBuiltWorld())
	}
	log.Printf("mock SatisfactoTerraform API listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, srv.Handler()))
}
