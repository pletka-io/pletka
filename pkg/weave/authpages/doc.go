// Package authpages owns the weave-native GET auth page surface. This
// package is a handler-only module: it renders pages through the shared
// weave templates renderer and owns no store.
//
// The matching JSON endpoints are mounted by pkg/auth. This package uses the
// weave shell plus auth.PrincipalFromContext.
package authpages
