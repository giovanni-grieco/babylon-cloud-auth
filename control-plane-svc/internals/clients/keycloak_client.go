package clients

import (
	"control-plane-svc/internals/domain"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	grant_type        = "client_credentials"
	authorizationPath = "/realms/babylon/protocol/openid-connect/token"
	usersPath         = "/admin/realms/babylon/users"
)

type AccessToken struct {
	TokenString    string
	ExpiresIn      time.Duration
	TimeOfIssuance time.Time
}

type KeycloakClient struct {
	AccessToken     *AccessToken
	KeycloakBaseUrl string
	ClientID        string
	ClientSecret    string
	RealmAlias      string
}

func NewKeycloakClient(KeycloakBaseUrl, ClientID, ClientSecret, RealmAlias string) *KeycloakClient {
	return &KeycloakClient{nil, KeycloakBaseUrl, ClientID, ClientSecret, RealmAlias}
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
	resp, err := http.PostForm(kc.KeycloakBaseUrl+authorizationPath, url.Values{"client_id": {kc.ClientID}, "client_secret": {kc.ClientSecret}, "grant_type": {grant_type}})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		fmt.Println(string(body))
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

func (kc *KeycloakClient) GetAllUsers() ([]domain.User, error) {
	var result []domain.User
	if kc.isAccessTokenExpired() {
		err := kc.acquireNewAccessToken()
		if err != nil {
			return result, err
		}
	}

	// we assume a valid access token has been acquire at this point

	request, err := http.NewRequest("GET", kc.KeycloakBaseUrl+usersPath, nil)
	if err != nil {
		return result, err
	}

	request.Header.Add("Authorization", "Bearer "+kc.AccessToken.TokenString)

	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		return result, err
	}
	defer response.Body.Close()

	if response.StatusCode == 200 {

		body, err := io.ReadAll(response.Body)
		if err != nil {
			return result, err
		}
		fmt.Println(string(body))
		var keycloakUserResult KeycloakUsersDto
		err = json.Unmarshal(body, &keycloakUserResult)
		if err != nil {
			return result, err
		}

		for _, kur := range keycloakUserResult {
			user := domain.User{Username: kur.Username, Email: kur.Email, RealmAlias: kc.RealmAlias}
			result = append(result, user)
		}
		//at this point we're all good!
	} else {
		return result, fmt.Errorf("Keycloak client did not manage to acquire users")
	}

	return result, nil

}
