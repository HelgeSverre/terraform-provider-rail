resource "rail_link" "quarterly_report" {
  url = "https://example.com/q3/synergy-report.pdf"
}

output "short_url" {
  value = rail_link.quarterly_report.short_url
}
