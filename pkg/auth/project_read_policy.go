package auth

import "context"

const projectResourceScope = "project"

// CheckProjectRead authorizes a current project access resource using the
// request's permissions. It can be injected into read-session source traversal;
// a captured pin or historical public visibility is not an access grant.
func CheckProjectRead(ctx context.Context, resource Resource) error {
	if resource.ScopeType != projectResourceScope || resource.ID == "" || !FromContext(ctx).Can(ProjectRead, resource, nil) {
		return &projectReadDenied{}
	}
	return nil
}

type projectReadDenied struct{}

func (*projectReadDenied) Error() string     { return "project read is not permitted" }
func (*projectReadDenied) IsForbidden() bool { return true }
