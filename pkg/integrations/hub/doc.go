// Package hub is the vertical slice that serves per-project integration
// configuration + actions. It owns the generic HTTP routes
//
//	GET  /projects/{projectID}/integrations/list-schema
//	GET  /projects/{projectID}/integrations/{id}/config-schema
//	PUT  /projects/{projectID}/integrations/{id}/config
//	POST /projects/{projectID}/integrations/{id}/actions/{actionID}
//
// and dispatches each request to the matching pkg/integrations/registry
// Integration. The hub never knows what an integration does — it only
// runs the contract: render the config form, validate + encrypt secrets,
// build the artifact provider, call RunAction, return ActionResultUI.
//
// New integrations are added by writing a package that implements
// registry.Integration and including it in the app's integration composition
// set. The hub picks them up automatically from the registry it is given.
package hub
