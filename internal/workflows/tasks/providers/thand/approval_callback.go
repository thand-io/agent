package thand

// approvalEventSource is the cloudevent source string used for approval events
// emitted by the agent. It matches the value used by createCallbackUrl in
// approvals_slack.go / approvals_email.go so that listener task filters and
// downstream consumers see a consistent source regardless of which channel
// (slack, email, local presence) produced the signal.
const approvalEventSource = "urn:thand:agent"
