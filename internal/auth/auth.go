package auth

import (
	"errors"
	"net/http"
	"strings"
)

// GetAPIKey extracts the API key from the header of the request.
// Example:
// Authorization: ApiKey 123456
func GetAPIKey(headers http.Header) (string, error) {
	authorization := headers.Get("Authorization")
	if authorization == "" {
		return "", errors.New("authorization header is missing")
	}

	authorizations := strings.Split(authorization, " ")
	if len(authorizations) != 2 {
		return "", errors.New("invalid authorization header")
	}

	if authorizations[0] != "ApiKey" {
		return "", errors.New("invalid authorization header")
	}

	return authorizations[1], nil

}
