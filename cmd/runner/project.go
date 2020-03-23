// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

// This file contains a simple project tracking value type that will accompany the
// contexts that are scoped to servicing a queue within a queue server

import (
	"context"
)

type projectContextKey string

type projectType string

var (
	projectKey = projectContextKey("project")
)

// NewContext returns a new Context that carries a value for the project associated with the context
func NewProjectContext(ctx context.Context, proj string) context.Context {
	return context.WithValue(ctx, projectKey, proj)
}

// FromContext returns the User value stored in ctx, if any.
func FromProjectContext(ctx context.Context) (proj string, wasPresent bool) {
	proj, wasPresent = ctx.Value(projectKey).(string)
	return proj, wasPresent
}
