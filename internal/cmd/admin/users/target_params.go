package users

import (
	"net/url"

	"github.com/antiwork/gumroad-cli/internal/cmd/admin/users/usertarget"
)

func userMutationParams(target userMutationTarget) url.Values {
	return usertarget.MutationParams(target)
}
