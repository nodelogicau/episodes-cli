package model

import (
	"fmt"
	"net/url"
	"sort"
)

// LinkRelationsDate is the date of the embedded snapshot of the IANA Link
// Relations registry (https://www.iana.org/assignments/link-relations/), the
// registry's last-modified date when it was taken. Updating the snapshot is a
// change to this file alone.
const LinkRelationsDate = "2026-10-13"

// registeredRels is the snapshot: every registered relation name, lowercase.
var registeredRels = []string{
	"about", "acl", "alternate", "amphtml", "api-catalog", "appendix", "apple-touch-icon",
	"apple-touch-startup-image", "archives", "author", "blocked-by", "bookmark",
	"c2pa-manifest", "canonical", "chapter", "cite-as", "collection",
	"compression-dictionary", "contents", "convertedfrom", "copyright", "create-form",
	"current", "deprecation", "describedby", "describes", "disclosure", "dns-prefetch",
	"dpp", "duplicate", "edit", "edit-form", "edit-media", "enclosure", "external", "first",
	"geofeed", "glossary", "help", "hosts", "hub", "ice-server", "icon", "index",
	"intervalafter", "intervalbefore", "intervalcontains", "intervaldisjoint",
	"intervalduring", "intervalequals", "intervalfinishedby", "intervalfinishes",
	"intervalin", "intervalmeets", "intervalmetby", "intervaloverlappedby",
	"intervaloverlaps", "intervalstartedby", "intervalstarts", "item", "last",
	"latest-version", "license", "linkset", "lrdd", "manifest", "mask-icon", "me",
	"media-feed", "memento", "micropub", "modulepreload", "monitor", "monitor-group",
	"next", "next-archive", "nofollow", "noopener", "noreferrer", "opener",
	"openid2.local_id", "openid2.provider", "original", "p3pv1", "payment", "pingback",
	"preconnect", "predecessor-version", "prefetch", "preload", "prerender", "prev",
	"prev-archive", "preview", "previous", "privacy-policy", "profile", "publication",
	"rdap-active", "rdap-bottom", "rdap-down", "rdap-top", "rdap-up", "related", "replies",
	"restconf", "ruleinput", "search", "section", "self", "service", "service-desc",
	"service-doc", "service-meta", "sip-trunking-capability", "sponsored", "start",
	"status", "stylesheet", "subsection", "successor-version", "sunset", "tag",
	"terms-of-service", "timegate", "timemap", "type", "ugc", "up", "version-history",
	"via", "webmention", "working-copy", "working-copy-of",
}

func init() { sort.Strings(registeredRels) }

// IsRegisteredRel reports whether rel is a lowercase registered name.
func IsRegisteredRel(rel string) bool {
	i := sort.SearchStrings(registeredRels, rel)
	return i < len(registeredRels) && registeredRels[i] == rel
}

// IsAbsoluteURI reports whether s is an absolute URI: a scheme and more.
func IsAbsoluteURI(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.IsAbs() && u.Opaque+u.Host+u.Path != ""
}

// RelProblem is why rel is not a valid link relation: neither a lowercase
// registered name nor an absolute URI.
func RelProblem(rel string) error {
	if IsRegisteredRel(rel) || IsAbsoluteURI(rel) {
		return nil
	}
	return fmt.Errorf("rel %q is neither a relation in the IANA Link Relations registry (snapshot %s) nor an absolute URI; use an absolute URI such as https://example.com/rels/%s to define your own", rel, LinkRelationsDate, url.PathEscape(rel))
}
