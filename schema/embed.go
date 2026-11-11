// Package schema embeds the canonical CUE schemas so Go code validates against
// the exact files `cue vet` checks. go:embed cannot reach parent directories,
// so the embed lives next to the .cue files.
package schema

import _ "embed"

// ReviewCUE is the source of review.cue.
//
//go:embed review.cue
var ReviewCUE string
