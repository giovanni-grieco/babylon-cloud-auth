package clients

import (
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
}

func NewKeycloakClient(KeycloakBaseUrl, ClientID, ClientSecret string) *KeycloakClient {
	return &KeycloakClient{nil, KeycloakBaseUrl, ClientID, ClientSecret}
}

type AccessTokenDto struct {
	AccessToken      string `json:"access_token"`
	ExpiresIn        int    `json:"expires_in"`
	RefreshExpiresIn int    `json:"refresh_expires_in"`
	TokenType        string `json:"token_type"`
	NotBeforePolicy  int    `json:"not-before-policy"`
	Scope            string `json:"scope"`
}

func (kc *KeycloakClient) AcquireNewAccessToken() error {
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
		json.Unmarshal(body, &accessTokenDto)

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

func (kc *KeycloakClient) IsAccessTokenExpired() bool {
	expirationTime := kc.AccessToken.TimeOfIssuance.Add(kc.AccessToken.ExpiresIn)
	return expirationTime.Before(time.Now())
}

func (kc *KeycloakClient) GetAllUsers() error {
	return fmt.Errorf("To be implemented\n")
}
