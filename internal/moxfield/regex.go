package moxfield

import "regexp"

// collectionPathRegex matches supported collection path formats and captures the ID.
var collectionPathRegex = regexp.MustCompile(`^/collection/([A-Za-z0-9_-]+)/?$`)

// collectionIDRegex validates a standalone collection ID: 1–32 alphanumeric, underscore, or hyphen characters.
var collectionIDRegex = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)
