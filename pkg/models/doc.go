// Package models holds shared, version-stable types for this notification service.
//
// These structs are safe to import from other services and client SDKs: JSON tags
// define the public wire shape; avoid breaking renames or tag changes without a
// semver bump of the consuming contract.
package models
