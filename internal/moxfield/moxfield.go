package moxfield

import "net/url"

// BuildCollectionURLFromID constructs a canonical collection URL from an ID.
func BuildCollectionURLFromID(collectionID string) string {
	return "https://moxfield.com/collection/" + collectionID
}

// IsValidCollectionURL reports whether the input is a supported Moxfield collection URL.
func IsValidCollectionURL(urlString string) bool {
	u, err := url.Parse(urlString)
	if err != nil {
		return false
	}
	if u.Scheme != "https" {
		return false
	}
	if u.Hostname() != "moxfield.com" && u.Hostname() != "www.moxfield.com" {
		return false
	}
	return collectionPathRegex.MatchString(u.EscapedPath())
}

// IsValidCollectionID reports whether id is a well-formed collection ID.
// Valid IDs are 1–32 characters composed of ASCII letters, digits, underscores, or hyphens.
func IsValidCollectionID(id string) bool {
	return collectionIDRegex.MatchString(id)
}

// ExtractCollectionIDFromURL returns the collection ID from a valid collection URL.
func ExtractCollectionIDFromURL(urlString string) string {
	u, err := url.Parse(urlString)
	if err != nil {
		return ""
	}
	parts := collectionPathRegex.FindStringSubmatch(u.EscapedPath())
	if len(parts) != 2 {
		return ""
	}
	return parts[1]
}
