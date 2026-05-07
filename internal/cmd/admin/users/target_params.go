package users

import "net/url"

func userMutationParams(target userMutationTarget) url.Values {
	params := url.Values{}
	params.Set("user_id", target.UserID)
	if target.ExpectedEmail != "" {
		params.Set("expected_email", target.ExpectedEmail)
	}
	return params
}
