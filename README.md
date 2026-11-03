<p align="center">
  <img src="https://raw.githubusercontent.com/HelgeSverre/rail-rs/main/.github/assets/header.jpg" alt="The world's first autonomous self-balancing link cloud. RAIL Enterprise Edition 26ai.">
</p>

<p align="center">
  <a href="https://github.com/HelgeSverre/terraform-provider-rail/actions/workflows/test.yml"><img alt="Tests" src="https://github.com/HelgeSverre/terraform-provider-rail/actions/workflows/test.yml/badge.svg"></a>
  <a href="LICENSE"><img alt="License: MIT" src="https://img.shields.io/badge/license-MIT-0b3d91"></a>
  <img alt="Terraform" src="https://img.shields.io/badge/terraform-%3E%3D1.12-7B42BC?logo=terraform&logoColor=white">
  <img alt="Plugin protocol" src="https://img.shields.io/badge/plugin%20protocol-v6-7B42BC">
  <br>
  <img alt="Links under management" src="https://img.shields.io/badge/links%20under%20management-declarative-success">
  <img alt="Data retention" src="https://img.shields.io/badge/data%20retention-indefinite-informational">
  <img alt="Drift" src="https://img.shields.io/badge/drift-detected%20and%20tolerated-success">
  <img alt="HashiCorp certification" src="https://img.shields.io/badge/HashiCorp%20certification-not%20applied%20for-lightgrey">
</p>

# Terraform Provider for RAIL

**Infrastructure as Code for Link Resilience.** This provider brings the
[RAIL](https://github.com/HelgeSverre/rail-rs) (Redundant Array of Inexpensive Links) platform under
declarative management. Short URLs can now be provisioned, reviewed in pull requests, planned,
applied, drift-detected and audited with the same rigor as the rest of your production
infrastructure.

> [!NOTE]
> Every short URL your organization distributes is now a Terraform resource. This was not previously
> possible, and it is not clear that it was previously necessary.

## Executive summary

| | |
| --- | --- |
| **Resources** | `rail_link` |
| **Data sources** | `rail_topology`, `rail_trace` |
| **Supported Terraform versions** | 1.12 and later, verified in CI against 1.12 and 1.17 |
| **Plugin protocol** | v6 (terraform-plugin-framework) |
| **Mean time to provision a link** | About 3 seconds through a six-tier pyramid |
| **Mean time to delete a link** | Not applicable. See [Data retention](#data-retention) |

## Usage

```hcl
terraform {
  required_providers {
    rail = {
      source = "helgesverre/rail"
    }
  }
}

provider "rail" {
  endpoint = "http://edge.rail.localhost:8080" # or RAIL_ENDPOINT
}

resource "rail_link" "quarterly_report" {
  url = "https://example.com/q3/synergy-report.pdf"
}

data "rail_trace" "quarterly_report" {
  link_id = rail_link.quarterly_report.id
}

# Continuous compliance: fail the run if a link drifts out of the browser redirect budget.
check "redirect_budget" {
  assert {
    condition     = data.rail_trace.quarterly_report.headroom >= 2
    error_message = "Link is ${data.rail_trace.quarterly_report.redirects} redirects deep; browsers give up at 20."
  }
}

output "short_url" {
  value = rail_link.quarterly_report.short_url
}
```

A complete reference deployment, covering a portfolio of links with redundancy and redirect-budget
checks against the docker compose Pyramid, is in [`examples/pyramid`](examples/pyramid/main.tf):

```
$ terraform apply
rail_link.campaign["roadmap"]: Creation complete after 3s [id=bcmSg6l]
rail_link.campaign["careers"]: Creation complete after 4s [id=Sy2IVhG]
rail_link.campaign["keynote"]: Creation complete after 4s [id=Aq7O4a5]

Apply complete! Resources: 3 added, 0 changed, 0 destroyed.

Outputs:

redirect_depth = {
  "careers" = 4
  "keynote" = 5
  "roadmap" = 4
}
```

## Reference

| Kind | Name | Purpose |
| --- | --- | --- |
| Resource | [`rail_link`](docs/resources/link.md) | A Link Resilience Unit: one destination, shortened by every vendor of the tier |
| Data source | [`rail_topology`](docs/data-sources/topology.md) | The tier and its direct upstreams, for audit purposes |
| Data source | [`rail_trace`](docs/data-sources/trace.md) | A link's redirect chain, hop by hop, with remaining redirect budget |

Existing links can be brought under management:

```sh
terraform import rail_link.quarterly_report dGJqXvP
```

## Lifecycle semantics

| Change | Plan | Effect on RAIL |
| --- | --- | --- |
| New `rail_link` | create | Link provisioned across every vendor. A destination that already has a link adopts it |
| `url` changed to a different destination | replace | New link provisioned; the old one is retained |
| `url` changed only in ways RAIL normalizes (host case, root slash) | update in-place | None. State records the new spelling |
| Mirror health changes between runs | none | Reflected in `mirrors` and `degraded` on refresh, without a diff |
| `terraform destroy` | delete | None. See below |

### Data retention

RAIL links are permanent, in keeping with industry-standard data retention practices. Destroying a
`rail_link` removes it from Terraform state and emits a warning; the link remains provisioned and
continues to resolve:

```
Warning: Link retained

RAIL links are permanent, in keeping with industry-standard data retention
practices. The link has been removed from Terraform state and remains
provisioned. It will continue to resolve.
```

This is the only correct behavior for printed collateral, which cannot be recalled by
`terraform destroy`.

## Installation

The provider is published under `helgesverre/rail`. To build it from source and use it without the
registry:

```sh
go install github.com/HelgeSverre/terraform-provider-rail@latest
```

```hcl
# ~/.terraformrc
provider_installation {
  dev_overrides {
    "helgesverre/rail" = "/Users/you/go/bin"
  }
  direct {}
}
```

## Development

```sh
go test ./...                      # unit tests
go generate ./...                  # regenerate docs/ from schema and examples/

# Acceptance tests provision real links against a real RAIL tier.
docker run --rm -d -p 8080:8080 -e RAIL_UPSTREAMS=postman-echo,httpbin.dev \
  ghcr.io/helgesverre/rail-rs:0.2.0
RAIL_ENDPOINT=http://localhost:8080 TF_ACC=1 go test ./... -v
```

The acceptance suite verifies creation, refresh, normalized updates, replacement, import and that
`terraform destroy` leaves the link resolving. CI runs it against Terraform 1.12 and 1.17, each with
a RAIL tier as a service container.

Releases are cut by pushing a `v*` tag. GoReleaser produces the archives, `SHA256SUMS` and the
registry manifest in the layout the Terraform Registry requires, and signs the checksums with the
key in the `GPG_PRIVATE_KEY` secret.

## License

[MIT](LICENSE).
