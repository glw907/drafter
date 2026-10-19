package blueprint

import "strings"

// BriefPath returns the repo-relative slash path of the brief for the page at pagePath: the
// page's blueprint path with .blueprint.yaml replaced by .brief.json, so the brief sits beside
// the blueprint.
func BriefPath(root, pagePath string) string {
	return strings.TrimSuffix(BlueprintPath(root, pagePath), blueprintSuffix) + ".brief.json"
}
