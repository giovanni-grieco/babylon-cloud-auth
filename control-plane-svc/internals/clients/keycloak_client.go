package clients

import (
	"control-plane-svc/internals/domain"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const (
	user_page_size    = 5
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

func (kc *KeycloakClient) RegisterUser(user *domain.User) error {
	return nil
}

func (kc *KeycloakClient) DeleteUser(user *domain.User) error {
	return nil
}
