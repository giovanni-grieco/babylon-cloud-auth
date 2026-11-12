package clients

import (
	"bytes"
	"context"
	"control-plane-svc/internals/domain"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"
)

const (
	user_page_size    = 100
	grant_type        = "client_credentials"
	authorizationPath = "/realms/babylon/protocol/openid-connect/token"
	usersPath         = "/admin/realms/babylon/users"
)

type AccessToken struct {
	TokenString    string
	ExpiresIn      time.Duration
	TimeOfIssuance time.Time
}

/*
Keycloak client is thread safe now!
*/
type KeycloakClient struct {
	mu              sync.Mutex
	AccessToken     *AccessToken
	KeycloakBaseUrl string
	ClientID        string
	ClientSecret    string
	RealmAlias      string
	httpClient      *http.Client
}

func NewKeycloakClient(KeycloakBaseUrl, ClientID, ClientSecret, RealmAlias string) *KeycloakClient {
	return &KeycloakClient{sync.Mutex{}, nil, KeycloakBaseUrl, ClientID, ClientSecret, RealmAlias, &http.Client{Timeout: 5 * time.Second}}
}

func NewKeycloakClientFromCfg(cfg RealmConfig) *KeycloakClient {
	return &KeycloakClient{
		sync.Mutex{},
		nil,
		cfg.BaseURL,
		cfg.ClientID,
		cfg.SecretEnv,
		cfg.Id,
		&http.Client{Timeout: 5 * time.Second},
	}
}

type AccessTokenDto struct {
	AccessToken      string `json:"access_token"`
	ExpiresIn        int    `json:"expires_in"`
	RefreshExpiresIn int    `json:"refresh_expires_in"`
	TokenType        string `json:"token_type"`
	NotBeforePolicy  int    `json:"not-before-policy"`
	Scope            string `json:"scope"`
}

type KeycloakUsersDto []KeycloakUserDto

type KeycloakUserDto struct {
	ID                         string                `json:"id"`
	Username                   string                `json:"username"`
	FirstName                  string                `json:"firstName"`
	LastName                   string                `json:"lastName"`
	Email                      string                `json:"email"`
	EmailVerified              bool                  `json:"emailVerified"`
	Enabled                    bool                  `json:"enabled"`
	CreatedTimestamp           int64                 `json:"createdTimestamp"`
	TOTP                       bool                  `json:"totp"`
	DisableableCredentialTypes []string              `json:"disableableCredentialTypes"`
	RequiredActions            []string              `json:"requiredActions"`
	NotBefore                  int64                 `json:"notBefore"`
	Access                     KeycloakUserAccessDto `json:"access"`
}

type KeycloakUserAccessDto struct {
	Manage bool `json:"manage"`
}

func (kc *KeycloakClient) acquireNewAccessToken() error {
	resp, err := kc.httpClient.PostForm(kc.KeycloakBaseUrl+authorizationPath, url.Values{"client_id": {kc.ClientID}, "client_secret": {kc.ClientSecret}, "grant_type": {grant_type}})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		//fmt.Println(string(body))
		var accessTokenDto AccessTokenDto
		err = json.Unmarshal(body, &accessTokenDto)
		if err != nil {
			return err
		}
		accessToken := &AccessToken{
			TokenString:    accessTokenDto.AccessToken,
			ExpiresIn:      time.Second * time.Duration(accessTokenDto.ExpiresIn),
			TimeOfIssuance: time.Now(),
		}
		kc.AccessToken = accessToken
	} else {
		return fmt.Errorf("Keycloak authorization did not succeed")
	}

	return nil
}

func (kc *KeycloakClient) isAccessTokenExpired() bool {
	if kc.AccessToken != nil {
		expirationTime := kc.AccessToken.TimeOfIssuance.Add(kc.AccessToken.ExpiresIn)
		return expirationTime.Before(time.Now())
	}
	return true
}

func (kc *KeycloakClient) getBearerToken() (string, error) {
	kc.mu.Lock()
	defer kc.mu.Unlock()

	if kc.isAccessTokenExpired() {
		err := kc.acquireNewAccessToken()
		if err != nil {
			return "", err
		}
	}

	return kc.AccessToken.TokenString, nil
}

func (kc *KeycloakClient) getAllUsersPage(first int) ([]domain.User, error) {
	var result []domain.User
	urlValues := url.Values{"max": {fmt.Sprintf("%d", user_page_size)}, "first": {fmt.Sprintf("%d", first)}}

	request, err := http.NewRequest("GET", kc.KeycloakBaseUrl+usersPath+"?"+urlValues.Encode(), nil)
	if err != nil {
		return result, err
	}

	bearerToken, err := kc.getBearerToken()
	if err != nil {
		return result, err
	}

	request.Header.Add("Authorization", "Bearer "+bearerToken)

	response, err := kc.httpClient.Do(request)
	if err != nil {
		return result, err
	}
	defer response.Body.Close()

	if response.StatusCode == 200 {

		body, err := io.ReadAll(response.Body)
		if err != nil {
			return result, err
		}
		//fmt.Println(string(body))
		var keycloakUserResult KeycloakUsersDto
		err = json.Unmarshal(body, &keycloakUserResult)
		if err != nil {
			return result, err
		}

		for _, kur := range keycloakUserResult {
			user := domain.User{RealmID: kur.ID, Username: kur.Username, Email: kur.Email, RealmAlias: kc.RealmAlias, State: domain.UNKNOWN}
			result = append(result, user)
		}
	} else {
		return result, fmt.Errorf("Keycloak client did not manage to acquire users")
	}

	return result, nil
}

func (kc *KeycloakClient) GetAllUsers() ([]domain.User, error) {
	var result []domain.User
	first := 0
	for {
		slog.Info("fetching users from keycloak", "realm", kc.RealmAlias, "first", first, "max", user_page_size)
		keycloakUserResult, err := kc.getAllUsersPage(first)
		if err != nil {
			return result, err
		}

		result = append(result, keycloakUserResult...)

		if len(keycloakUserResult) < user_page_size {
			break // No more users to fetch, exit the loop
		} else {
			first += user_page_size // Move to the next page
		}
		//at this point we're all good!
	}
	return result, nil

}

// ErrConflict is returned by CreateUser when Keycloak answers 409: a user with
// the same username or email already exists in the realm.
var ErrConflict = errors.New("keycloak: user with same username or email already exists")

// HTTPError is an unexpected status code returned by the Keycloak Admin API.
type HTTPError struct {
	Op         string
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("keycloak %s: unexpected status %d: %s", e.Op, e.StatusCode, e.Body)
}

// IsTransient reports whether retrying the same call later may succeed:
// network errors, timeouts, token acquisition failures, 5xx and 429.
// Any other 4xx (and ErrConflict) is permanent.
func IsTransient(err error) bool {
	if err == nil || errors.Is(err, ErrConflict) {
		return false
	}
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode >= 500 || httpErr.StatusCode == http.StatusTooManyRequests
	}
	return true
}

type keycloakCredentialDto struct {
	Type      string `json:"type"`
	Value     string `json:"value"`
	Temporary bool   `json:"temporary"`
}

type keycloakCreateUserDto struct {
	Username    string                  `json:"username"`
	Email       string                  `json:"email"`
	Enabled     bool                    `json:"enabled"`
	Credentials []keycloakCredentialDto `json:"credentials,omitempty"`
}

func (kc *KeycloakClient) do(ctx context.Context, method, requestPath string, body any) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(payload)
	}

	request, err := http.NewRequestWithContext(ctx, method, kc.KeycloakBaseUrl+requestPath, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	bearerToken, err := kc.getBearerToken()
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+bearerToken)

	return kc.httpClient.Do(request)
}

func unexpectedStatus(op string, response *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
	return &HTTPError{Op: op, StatusCode: response.StatusCode, Body: string(body)}
}

// FindUserByUsername returns the realm user with exactly this username
// (RealmID holds the Keycloak id), or nil, nil when it does not exist.
func (kc *KeycloakClient) FindUserByUsername(ctx context.Context, username string) (*domain.User, error) {
	query := url.Values{"username": {username}, "exact": {"true"}}
	response, err := kc.do(ctx, http.MethodGet, usersPath+"?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, unexpectedStatus("find user", response)
	}

	var users KeycloakUsersDto
	if err := json.NewDecoder(response.Body).Decode(&users); err != nil {
		return nil, err
	}
	for _, kur := range users {
		if strings.EqualFold(kur.Username, username) {
			return &domain.User{RealmID: kur.ID, Username: kur.Username, Email: kur.Email, RealmAlias: kc.RealmAlias, State: domain.UNKNOWN}, nil
		}
	}
	return nil, nil
}

// CreateUser creates an enabled user with a temporary password (Keycloak
// forces a change at first login) and returns its Keycloak id.
// It returns ErrConflict when the username or the email is already taken.
func (kc *KeycloakClient) CreateUser(ctx context.Context, username, email, temporaryPassword string) (string, error) {
	dto := keycloakCreateUserDto{
		Username: username,
		Email:    email,
		Enabled:  true,
		Credentials: []keycloakCredentialDto{
			{Type: "password", Value: temporaryPassword, Temporary: true},
		},
	}

	response, err := kc.do(ctx, http.MethodPost, usersPath, dto)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	switch response.StatusCode {
	case http.StatusCreated:
		location := response.Header.Get("Location")
		if location == "" {
			return "", fmt.Errorf("keycloak create user: missing Location header")
		}
		return path.Base(location), nil
	case http.StatusConflict:
		return "", ErrConflict
	default:
		return "", unexpectedStatus("create user", response)
	}
}

// ResetTemporaryPassword replaces the user's password with a temporary one.
func (kc *KeycloakClient) ResetTemporaryPassword(ctx context.Context, keycloakID, temporaryPassword string) error {
	credential := keycloakCredentialDto{Type: "password", Value: temporaryPassword, Temporary: true}
	response, err := kc.do(ctx, http.MethodPut, usersPath+"/"+url.PathEscape(keycloakID)+"/reset-password", credential)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusNoContent {
		return unexpectedStatus("reset password", response)
	}
	return nil
}

// DeleteUser deletes the user. A user that does not exist (404) counts as
// deleted, so the call is safe to repeat.
func (kc *KeycloakClient) DeleteUser(ctx context.Context, keycloakID string) error {
	response, err := kc.do(ctx, http.MethodDelete, usersPath+"/"+url.PathEscape(keycloakID), nil)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	switch response.StatusCode {
	case http.StatusNoContent, http.StatusNotFound:
		return nil
	default:
		return unexpectedStatus("delete user", response)
	}
}
