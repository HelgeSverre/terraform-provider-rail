# Provision a portfolio of Link Resilience Units against the reference Pyramid
# (docker compose up in the rail-rs repository), with continuous compliance checks.

terraform {
  required_providers {
    rail = {
      source = "helgesverre/rail"
    }
  }
}

provider "rail" {
  endpoint = "http://edge.rail.localhost:8080"
}

variable "campaign_urls" {
  type = map(string)
  default = {
    keynote = "https://www.rust-lang.org/"
    roadmap = "https://github.com/HelgeSverre/rail-rs"
    careers = "https://example.com/careers/link-resilience-engineer"
  }
}

resource "rail_link" "campaign" {
  for_each = var.campaign_urls
  url      = each.value
}

data "rail_trace" "campaign" {
  for_each = rail_link.campaign
  link_id  = each.value.id
}

check "redirect_budget" {
  assert {
    condition     = alltrue([for t in data.rail_trace.campaign : t.headroom >= 2])
    error_message = "A campaign link is too close to the browser redirect limit."
  }
}

check "redundancy" {
  assert {
    condition     = alltrue([for l in rail_link.campaign : !l.degraded])
    error_message = "A campaign link is in Degraded Mode."
  }
}

output "short_urls" {
  value = { for k, l in rail_link.campaign : k => l.short_url }
}

output "redirect_depth" {
  value = { for k, t in data.rail_trace.campaign : k => t.redirects }
}
