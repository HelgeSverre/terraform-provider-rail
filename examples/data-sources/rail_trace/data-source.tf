data "rail_trace" "quarterly_report" {
  link_id = rail_link.quarterly_report.id
}

# Fail the run if a link drifts out of the browser redirect budget.
check "redirect_budget" {
  assert {
    condition     = data.rail_trace.quarterly_report.headroom >= 2
    error_message = "Link is ${data.rail_trace.quarterly_report.redirects} redirects deep; browsers give up at 20."
  }
}
