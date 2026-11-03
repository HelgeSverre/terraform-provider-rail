package provider

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/HelgeSverre/terraform-provider-rail/internal/rail"
)

// Acceptance tests run against a real RAIL tier at RAIL_ENDPOINT, e.g.
//
//	docker run -p 8080:8080 -e RAIL_UPSTREAMS=postman-echo ghcr.io/helgesverre/rail-rs:0.2.0
//	RAIL_ENDPOINT=http://localhost:8080 TF_ACC=1 go test ./...
var protoV6 = map[string]func() (tfprotov6.ProviderServer, error){
	"rail": providerserver.NewProtocol6WithError(New("test")()),
}

func preCheck(t *testing.T) {
	if os.Getenv("RAIL_ENDPOINT") == "" {
		t.Fatal("RAIL_ENDPOINT must point at a RAIL tier for acceptance tests")
	}
}

func TestAccLink(t *testing.T) {
	// Unique per run: RAIL reuses links per destination, and links are permanent.
	dest := fmt.Sprintf("https://example.com/terraform-acc/%d", rand.Uint64())
	config := func(url string) string {
		return fmt.Sprintf(`
resource "rail_link" "test" {
  url = %q
}

data "rail_trace" "test" {
  link_id = rail_link.test.id
}
`, url)
	}

	var id string
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { preCheck(t) },
		ProtoV6ProviderFactories: protoV6,
		Steps: []resource.TestStep{
			{
				Config: config(dest),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rail_link.test", "target", dest),
					resource.TestCheckResourceAttrSet("rail_link.test", "id"),
					resource.TestCheckResourceAttrWith("rail_link.test", "short_url", func(v string) error {
						if !strings.HasPrefix(v, "http") {
							return fmt.Errorf("short_url %q is not a URL", v)
						}
						return nil
					}),
					resource.TestCheckResourceAttrSet("rail_link.test", "compression_ratio"),
					resource.TestCheckResourceAttrWith("data.rail_trace.test", "redirects", func(v string) error {
						if v == "0" {
							return fmt.Errorf("a RAIL link should redirect at least once")
						}
						return nil
					}),
					resource.TestCheckResourceAttr("data.rail_trace.test", "gave_up", "false"),
					func(s *terraform.State) error {
						id = s.RootModule().Resources["rail_link.test"].Primary.ID
						return nil
					},
				),
			},
			{
				// RAIL normalizes host case: same destination, so update state, keep the link.
				Config: config(strings.Replace(dest, "example.com", "EXAMPLE.com", 1)),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("rail_link.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.TestCheckResourceAttrWith("rail_link.test", "id", func(v string) error {
					if v != id {
						return fmt.Errorf("id changed from %s to %s: the link was replaced", id, v)
					}
					return nil
				}),
			},
			{
				// A different destination is a different link.
				Config: config(dest + "-v2"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("rail_link.test", plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("rail_link.test", "target", dest+"-v2"),
					func(s *terraform.State) error {
						id = s.RootModule().Resources["rail_link.test"].Primary.ID
						return nil
					},
				),
			},
			{
				ResourceName:      "rail_link.test",
				ImportState:       true,
				ImportStateVerify: true,
				// Import only knows RAIL's normalized target, not how the config spelled it.
				ImportStateVerifyIgnore: []string{"url", "degraded", "mirrors"},
			},
		},
		// Links are permanent: destroying the resource must leave the link resolving.
		CheckDestroy: func(*terraform.State) error {
			client, err := rail.New(os.Getenv("RAIL_ENDPOINT"))
			if err != nil {
				return err
			}
			if _, err := client.Link(context.Background(), id); err != nil {
				return fmt.Errorf("link %s should outlive terraform destroy: %w", id, err)
			}
			return nil
		},
	})
}

func TestAccTopology(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { preCheck(t) },
		ProtoV6ProviderFactories: protoV6,
		Steps: []resource.TestStep{{
			Config: `data "rail_topology" "this" {}`,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttrSet("data.rail_topology.this", "id"),
				resource.TestCheckResourceAttrSet("data.rail_topology.this", "instance"),
				resource.TestCheckResourceAttrSet("data.rail_topology.this", "public_url"),
			),
		}},
	})
}

func TestNormalize(t *testing.T) {
	for a, b := range map[string]string{
		"https://Example.COM":       "https://example.com/",
		"HTTPS://example.com/a?b=c": "https://example.com/a?b=c",
		"https://example.com/path/": "https://example.com/path/",
	} {
		if normalize(a) != normalize(b) {
			t.Errorf("normalize(%q)=%q, normalize(%q)=%q", a, normalize(a), b, normalize(b))
		}
	}
	if normalize("https://example.com/a") == normalize("https://example.com/b") {
		t.Error("different paths must not normalize together")
	}
}
