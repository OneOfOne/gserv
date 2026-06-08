// Package router is the main routing package of gserv.
//
// It provides an efficient HTTP request router with support for path parameters,
// OpenAPI/Swagger auto-generation, panic recovery, and method fallback (HEAD→GET).
//
// # Basic Usage
//
//	r := router.New(nil)
//	r.AddRoute("users", "GET", "/users/:id", func(w http.ResponseWriter, req *http.Request, p router.Params) {
//	    id := p.Get("id")
//	    w.Write([]byte(id))
//	})
//	http.ListenAndServe(":8080", r)
//
// # Route Parameters
//
// Two path parameter syntaxes are supported:
//
//   - :name — single-segment parameter (e.g. /users/:id matches /users/42)
//   - *name — multi-segment wildcard parameter, must be the last segment
//     (e.g. /files/*path matches /files/a/b/c.txt with path="a/b/c.txt")
//
// Only one wildcard (*) parameter is allowed per route. It must appear as the
// final path component.
//
// # Options
//
// The router accepts an optional *router.Options struct:
//
//   - OnRequestDone — callback invoked after a matched request completes, receiving
//     context, group name, method, URI, and duration. Useful for metrics/logging.
//   - APIInfo — OpenAPI info object (title, description, version).
//   - NoAutoCleanURL — skip automatic URL path cleaning (e.g. /a/../b → /b).
//   - NoDefaultPanicHandler — do not set the default panic recovery handler.
//   - NoPanicOnInvalidAddRoute — return an error instead of panicking when
//     AddRoute receives an invalid route pattern.
//   - CatchPanics — enable panic recovery (on by default).
//   - NoAutoHeadToGet — disable automatic HEAD → GET fallback.
//   - ProfileLabels — add pprof labels (group, method, uri) to the goroutine context.
//   - AutoGenerateSwagger — automatically generate OpenAPI documentation for routes.
//
// # Groups
//
// Each route is assigned a group name (first parameter to AddRoute). Groups are
// used for categorization in the router and exposed via the OnRequestDone callback
// and pprof labels when ProfileLabels is enabled.
//
// # Route Retrieval
//
// The currently matched route can be retrieved inside any handler via:
//
//	route := router.RouteFromRequest(req)
//
// # Params
//
// Route parameters are provided as a router.Params slice, keyed by parameter name.
// The Params type is NOT safe for use outside the handler — if you need to store
// it, call params.Copy(). Two helper methods are available:
//
//   - Get(name) — retrieve a parameter value by name.
//   - GetExt(name) — split the parameter at its last extension (e.g. "report.json" → ("report", "json")).
//
// # Swagger/OpenAPI Support
//
// Routes can be documented using the WithDoc method:
//
//	r.AddRoute("users", "GET", "/users/:id", handler).WithDoc("Get user by ID", true)
//
// The second argument enables automatic parameter generation from route definitions.
// Further customization is available via fluent builder methods on *SwaggerRoute:
//
//   - WithOperationID, WithSummary, WithDescription, WithTags
//   - WithParam(name, desc, in, typ, required, schema) — add a parameter
//   - WithBody(contentType, example) — document request body
//   - WithResponse(statusCode, description) — document response
//   - WithExample(name, desc) — add an example
//   - AsPublic() — mark route documentation as public
//
// The full OpenAPI spec can be retrieved via router.Swagger().
//
// # Disabling Routes
//
// Individual routes can be disabled at runtime without removing them:
//
//	r.DisableRoute("GET", "/users/:id", true)
//
// # Method Fallback
//
// By default, if no handler is registered for HEAD requests on a path, the router
// automatically falls back to the GET handler for that path and discards the body.

package router
