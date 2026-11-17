package refs

import (
	"regexp"
	"strings"
)

var doiPattern = regexp.MustCompile(`^10\.\d{4,9}/\S+$`)
var arxivPattern = regexp.MustCompile(`^(\d{4}\.\d{4,5}|[a-z-]+(\.[A-Z]{2})?/\d{7})(v\d+)?$`)

// NormalizeDOI accepts the bare identifier and its usual DOI URL/prefix forms.
func NormalizeDOI(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	for _, prefix := range []string{"https://doi.org/", "http://doi.org/", "https://dx.doi.org/", "http://dx.doi.org/", "doi:"} {
		s = strings.TrimPrefix(s, prefix)
	}
	if !doiPattern.MatchString(s) {
		return ""
	}
	return s
}

// IsArxivID is shared by bibliography checks and the arXiv client.
func IsArxivID(s string) bool { return arxivPattern.MatchString(strings.TrimSpace(s)) }

// MetadataIdentifier is an identifier both the report and metadata endpoint
// can use. Invalid fields don't offer an action which the endpoint rejects.
func MetadataIdentifier(fields map[string]string) string {
	if doi := fields["doi"]; doi != "" {
		if doi = NormalizeDOI(doi); doi != "" {
			return "doi:" + doi
		}
		return ""
	}
	if strings.EqualFold(fields["archiveprefix"], "arxiv") || strings.EqualFold(fields["eprinttype"], "arxiv") {
		id := strings.TrimSpace(fields["eprint"])
		if len(id) >= 6 && strings.EqualFold(id[:6], "arxiv:") {
			id = strings.TrimSpace(id[6:])
		}
		if IsArxivID(id) {
			return "arxiv:" + id
		}
	}
	return ""
}
