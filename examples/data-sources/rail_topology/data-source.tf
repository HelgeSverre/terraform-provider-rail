data "rail_topology" "edge" {}

output "edge_upstreams" {
  value = data.rail_topology.edge.upstreams
}
