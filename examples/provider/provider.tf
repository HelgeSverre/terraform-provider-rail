terraform {
  required_providers {
    rail = {
      source = "helgesverre/rail"
    }
  }
}

# The top of the pyramid. Also settable as RAIL_ENDPOINT.
provider "rail" {
  endpoint = "http://edge.rail.localhost:8080"
}
