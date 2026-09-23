package httpapi

import (
	"net/http"
	"strings"

	"corerp.local/backend/internal/core"
)

type Authenticator interface {
	Authenticate(*http.Request) (string, error)
}

type StaticTokenAuthenticator struct {
	principalsByToken map[string]string
}

func NewStaticTokenAuthenticator(tokens map[string]string) (*StaticTokenAuthenticator, error) {
	if len(tokens) == 0 {
		return nil, core.NewError(core.CodeInvalidArgument, "at least one authentication token is required")
	}
	copyOfTokens := make(map[string]string, len(tokens))
	for token, principalID := range tokens {
		if strings.TrimSpace(token) == "" || strings.TrimSpace(principalID) == "" {
			return nil, core.NewError(core.CodeInvalidArgument, "authentication tokens and principal IDs must be nonempty")
		}
		copyOfTokens[token] = principalID
	}
	return &StaticTokenAuthenticator{principalsByToken: copyOfTokens}, nil
}

func (a *StaticTokenAuthenticator) Authenticate(request *http.Request) (string, error) {
	header := strings.TrimSpace(request.Header.Get("Authorization"))
	scheme, credential, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(credential) == "" {
		return "", core.NewError(core.CodeUnauthenticated, "a Bearer credential is required")
	}
	principalID, ok := a.principalsByToken[strings.TrimSpace(credential)]
	if !ok {
		return "", core.NewError(core.CodeUnauthenticated, "the Bearer credential is invalid")
	}
	return principalID, nil
}
