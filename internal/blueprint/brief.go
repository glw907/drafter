package blueprint

import (
	"path"
	"strings"
)

// BriefPath returns the repo-relative slash path of the brief for the page at pagePath. With an
// empty template, the brief sits beside the blueprint: the page's blueprint path with
// .blueprint.yaml replaced by .brief.json. Otherwise template is a path relative to the project
// root in which {dir} stands for the base name of the page's directory and {name} for the page's
// file name without .md.
func BriefPath(root, template, pagePath string) string {
	if template == "" {
		return strings.TrimSuffix(BlueprintPath(root, pagePath), blueprintSuffix) + ".brief.json"
	}
	r := strings.NewReplacer("{dir}", path.Base(path.Dir(pagePath)), "{name}", strings.TrimSuffix(path.Base(pagePath), ".md"))
	return path.Clean(r.Replace(template))
}
